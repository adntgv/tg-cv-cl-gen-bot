from telegram import Update
from telegram.ext import ApplicationBuilder, CommandHandler, ContextTypes
import openai
import json
import os
from dotenv import load_dotenv

load_dotenv()

# Retrieve environment variables using os.getenv
LLM_API_KEY = os.getenv('LLM_API_KEY')
LLM_API_URL = os.getenv('LLM_API_URL')
LLM_MODEL = os.getenv('LLM_MODEL')
TELEGRAM_BOT_TOKEN = os.getenv('TELEGRAM_BOT_TOKEN')


# Check if all required variables are loaded
if not all([LLM_API_KEY, LLM_API_URL, LLM_MODEL, TELEGRAM_BOT_TOKEN]):
    raise ValueError("Some environment variables are missing")

openai.api_key = LLM_API_KEY
openai.base_url = LLM_API_KEY

RESUMES_FILE = "resumes.json"

def add_resume_to_file(user_id: str, resume_text: str) -> None:
    """Add a new resume to the JSON file."""
    resumes = load_resumes_from_file()
    resumes[user_id] = resume_text
    save_resumes_to_file(resumes)

def load_resumes_from_file() -> dict:
    """Load resumes from the JSON file."""
    try:
        with open(RESUMES_FILE, 'r') as file:
            return json.load(file)
    except FileNotFoundError:
        return {}

def save_resumes_to_file(resumes: dict) -> None:
    """Save resumes to the JSON file."""
    with open(RESUMES_FILE, 'w') as file:
        json.dump(resumes, file, indent=4)

async def setup(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    """Handles the /setup command, storing the user's resume in a dictionary with a unique ID."""
    
    try:
        global resumes
        resume = update.message.text.split("/setup", 1)[1].strip()  # Extract job description

        if resume:
            resume_id = update.effective_user.id  # Generate a unique ID for the resume
            add_resume_to_file(resume_id, resume)
            print(update.message.text)
            await context.bot.send_message(chat_id=update.effective_chat.id, text=f"Resume successfully stored with ID: {resume_id}")
        else:
            await context.bot.send_message(chat_id=update.effective_chat.id, text="Please send your resume as a text message. \nLike '/setup ....'")
    except Exception as e: 
        print("setup Exception: ", e) 
        await context.bot.send_message(chat_id=update.effective_chat.id, text="Error happened with your request")

async def generate(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    """Handles the /generate command, creating a new resume and cover letter based on the provided job description."""
    resumes = load_resumes_from_file()

    resume_id = str(update.effective_user.id)
    if resume_id in resumes.keys():
        try:
            resume = resumes[resume_id]
            parts = update.message.text.split(" ", 1)
            if len(parts) < 2:
                await context.bot.send_message(chat_id=update.effective_chat.id, text="Please provide job description too")
                return
            
            job_description = parts[1].strip()

            completion = openai.chat.completions.create(
                model= LLM_MODEL,
                messages=[
                    {"role": "user", "content": f"I will provide you with resume and job description. Generate a new resume based on my resume to fit the job description. {resume} {job_description}"},
                ],
            )

            gen_resume = completion.choices[0].message.content

            completion = openai.chat.completions.create(
                model= LLM_MODEL,
                messages=[
                    {"role": "user", "content": f"I will provide you with resume and job description. Generate a new cover letter based on my resume to fit the job description. {resume} {job_description}"},
                ],
            )

            gen_cover_letter = completion.choices[0].message.content

            generated_resume = "Here's your tailored resume based on the provided job description:\n\n" + \
                            gen_resume  # ... (generated resume content)
            generated_cover_letter = "Here's your cover letter tailored to the same job description:\n\n" + \
                            gen_cover_letter        # ... (generated cover letter content)

            await context.bot.send_message(chat_id=update.effective_chat.id, text=generated_resume)
            await context.bot.send_message(chat_id=update.effective_chat.id, text=generated_cover_letter)
        except Exception as e:
            print("setup Exception: ", e) 
            await context.bot.send_message(chat_id=update.effective_chat.id, text="Error happened with your request")
    else:
        await context.bot.send_message(chat_id=update.effective_chat.id, text="Please provide your resume with /setup.")


async def hello(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    """Greets the user."""
    
    await update.message.reply_text(f'Hello {update.effective_user.name}')

async def start(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    """Greets the user."""
    
    await update.message.reply_text(f'Hello {update.effective_user.name}, please provide you resume via command /setup ...')


def generate_unique_id():
    """Generates a unique ID for each resume."""
    import uuid
    return str(uuid.uuid4())[:8]  # Use the first 8 characters of a UUID for a shorter ID

app = ApplicationBuilder().token(TELEGRAM_BOT_TOKEN).build()

app.add_handler(CommandHandler("start", start))
app.add_handler(CommandHandler("hello", hello))
app.add_handler(CommandHandler("setup", setup))
app.add_handler(CommandHandler("generate", generate))

app.run_polling()