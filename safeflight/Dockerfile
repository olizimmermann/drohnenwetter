# Use the official Python image from the Docker Hub
FROM python:3.9-slim

# Set environment variables for Python
ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1

# Set the working directory in the container
WORKDIR /app

# Copy requirements first for dependency installation caching
COPY requirements.txt /app/requirements.txt

# Install necessary system dependencies and Python dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && pip install --no-cache-dir -r requirements.txt \
    && apt-get remove -y build-essential \
    && apt-get autoremove -y \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Copy the application code into the container
COPY . /app

# Create a non-root user and set ownership of relevant directories
RUN useradd --create-home --shell /bin/bash --uid 1001 safeflight && \
    chown -R safeflight:safeflight /app

# Create a directory for logs and set ownership
RUN mkdir -p /var/log/safeflight && \
    touch /var/log/safeflight/log.log && \
    chown -R safeflight:safeflight /var/log/safeflight

# Switch to the non-root user
USER safeflight

# Run the application
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000", "--workers", "4"]
