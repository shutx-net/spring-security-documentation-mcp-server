FROM python:3.12-slim
RUN useradd -u 1000 -r nonroot
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY indexer.py .
USER nonroot
ENTRYPOINT ["python", "indexer.py"]
