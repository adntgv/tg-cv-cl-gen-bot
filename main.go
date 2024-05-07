package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	openai "github.com/sashabaranov/go-openai"

	_ "github.com/joho/godotenv/autoload"
	"github.com/schollz/jsonstore"
)

// Send any text message to the bot after the bot has been started

// type tgbot struct {
// 	b *bot.Bot
// }

// type gpt struct {
// 	client *openai.Client
// }

// type App struct {
// 	tgbot *tgbot
// 	gpt   *gpt
// }

var (
	client    *openai.Client
	LLM_MODEL string
)

func main() {
	LLM_API_KEY := os.Getenv("LLM_API_KEY")
	LLM_API_URL := os.Getenv("LLM_API_URL")
	LLM_MODEL = os.Getenv("LLM_MODEL")
	TELEGRAM_BOT_TOKEN := os.Getenv("TELEGRAM_BOT_TOKEN")

	config := openai.DefaultConfig(LLM_API_KEY)
	config.BaseURL = LLM_API_URL
	client = openai.NewClientWithConfig(config)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	opts := []bot.Option{
		bot.WithDefaultHandler(handler),
	}

	b, err := bot.New(TELEGRAM_BOT_TOKEN, opts...)
	if err != nil {
		panic(err)
	}

	b.RegisterHandler(bot.HandlerTypeMessageText, "/hello", bot.MatchTypeExact, helloHandler)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/setup", bot.MatchTypePrefix, setupHandler)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/generate", bot.MatchTypePrefix, generateHandler)

	createResumesFileIfNotExists()

	go b.Start(ctx)

	port := os.Getenv("PORT")

	if port == "" {
		port = "8000"
	}

	http.HandleFunc("/", HelloHandler)

	log.Println("Listening on port", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}

func createResumesFileIfNotExists() {
	f, err := os.OpenFile("resumes.json", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	f.Close()
}

func HelloHandler(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprintf(w, "Hello from Koyeb\n")
}

func helloHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      "Hello, *" + bot.EscapeMarkdown(update.Message.From.FirstName) + "*",
		ParseMode: models.ParseModeMarkdown,
	})
}

func setupHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	go func() {
		parts := strings.Split(update.Message.Text, "/setup")
		if len(parts) < 2 {
			respond(ctx, update, b, "please provide your resume too. Like '/setup ...'")
			return
		}

		if len(parts[1]) < 1 {
			respond(ctx, update, b, "resume too short")
			return
		}

		resumeText := parts[1]
		userId := update.Message.From.ID

		saveResume(userId, resumeText)

		respond(ctx, update, b, "resume saved")
	}()
}

func respond(ctx context.Context, update *models.Update, b *bot.Bot, message string) {
	log.Println("respond", message)
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   message,
	})
}

func generateHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	go func() {
		parts := strings.Split(update.Message.Text, "/generate")
		if len(parts) < 2 {
			respond(ctx, update, b, "please provide job description too. Like '/generate ...'")
			return
		}

		if len(parts[1]) < 1 {
			respond(ctx, update, b, "job description too short")
			return
		}

		jobDescription := parts[1]
		userId := update.Message.From.ID

		resume := getResume(userId)
		if len(resume) < 1 {
			respond(ctx, update, b, "resume is not provided. Do /setup first")
			return
		}

		respond(ctx, update, b, "generating resume")
		gResume := generateResume(jobDescription, resume)
		respond(ctx, update, b, gResume)

		respond(ctx, update, b, "generating cover letter")
		gCoverLetter := generateCoverLetter(jobDescription, gResume)
		respond(ctx, update, b, gCoverLetter)
	}()
}

func handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   update.Message.Text,
	})
}

var template = "Generate %v in simple text, but with spaces, comas and indents and etc from this job description and resume: JOB DESCRIPTION: %v; RESUME:%v. Be sure to fill all placeholders, or out request won't be accepted. Make it short, and straigh to the point"

func generateResume(jd string, cv string) string {
	request := fmt.Sprintf(template, "new tailored resume", jd, cv)

	res, err := getCompletion(request)
	if err != nil {
		log.Println("generateResume", err)
		return "could not generate resume" + err.Error()
	}

	return res
}
func generateCoverLetter(jd string, cv string) string {
	request := fmt.Sprintf(template, "tailored cover letter", jd, cv)

	res, err := getCompletion(request)
	if err != nil {
		log.Println("generateCoverLetter", err)
		return "could not generate cover letter " + err.Error()
	}

	return res
}

func getCompletion(request string) (string, error) {
	req := openai.ChatCompletionRequest{
		Model: LLM_MODEL,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: request,
			},
		},
		Stream: true,
	}

	ctx := context.Background()

	stream, err := client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		fmt.Printf("ChatCompletionStream error: %v\n", err)
		return "", fmt.Errorf("chatCompletion error: %v", err)
	}

	defer stream.Close()

	fmt.Printf("Stream response: ")
	resp := ""
	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			fmt.Println("\nStream finished")
			break
		}

		if err != nil {
			fmt.Printf("\nStream error: %v\n", err)
			return resp, fmt.Errorf("chatCompletion error: %v", err)
		}

		resp += response.Choices[0].Delta.Content
		fmt.Printf(response.Choices[0].Delta.Content)
	}

	return resp, nil
}

func saveResume(userId int64, resumeText string) {
	// Load any JSON / GZipped JSON
	ks, err := jsonstore.Open("resumes.json")
	if err != nil {
		panic(err)
	}

	err = ks.Set(toKey(userId), resumeText)
	if err != nil {
		panic(err)
	}

	// Saving will automatically gzip if .gz is provided
	if err = jsonstore.Save(ks, "resumes.json"); err != nil {
		panic(err)
	}
}

func toKey(userId int64) string {
	return fmt.Sprintf("%v", userId)
}

func getResume(userId int64) (resumeText string) {
	// Load any JSON / GZipped JSON
	ks, err := jsonstore.Open("resumes.json")
	if err != nil {
		panic(err)
	}

	err = ks.Get(toKey(userId), &resumeText)
	if err != nil {
		panic(err)
	}

	return
}
