# Imagen de los agentes Pydantic AI de la demostración.
# python:3.13.16-slim
FROM python@sha256:bf44cdfcb76cd3b41e879bc058fc37ec5872002ccfde7fcb765e218cde0cd79c
LABEL maintainer="Autores del plugin A2A para KrakenD"
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PYDANTIC_AI_NO_BANNER=1
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY demo ./demo
USER 65534:65534
EXPOSE 8000
CMD ["python", "-m", "demo.servidor"]
