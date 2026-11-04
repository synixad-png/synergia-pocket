FROM python:3.12-slim
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY app.py grade_features.py demo.py session_store.py message_features.py push_features.py school_files.py message_attachments.py canteen_sp3.py canteen_calendar.py canteen_payments.py ./
COPY data ./data
COPY static ./static
RUN useradd --create-home --uid 10001 webuser
USER webuser
EXPOSE 10000
CMD ["sh", "-c", "exec uvicorn app:app --host 0.0.0.0 --port ${PORT:-10000} --proxy-headers"]

