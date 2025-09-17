from pydantic import BaseModel

class LogRequest(BaseModel):
    text: str

class LogResponse(BaseModel):
    label: str
    score: float
