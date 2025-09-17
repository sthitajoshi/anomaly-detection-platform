from transformers import pipeline
import torch

MODEL_NAME = "Dumi2025/log-anomaly-detection-model-roberta"

# Check if GPU is available
device = 0 if torch.cuda.is_available() else -1

pipe = pipeline("text-classification", model=MODEL_NAME, device=device)

def get_model():
    return pipe
