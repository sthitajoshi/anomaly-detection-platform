from fastapi import FastAPI
from app.schemas import LogRequest, LogResponse
from .interface import predict_log

app = FastAPI(title="Log Anomaly Detection Service")

@app.post("/predict", response_model=LogResponse)
def predict(request: LogRequest):
    result = predict_log(request.text)
    return LogResponse(label=result["label"], score=result["score"])
