from app.model import get_model

pipe = get_model()  # load model once at import

def predict_log(text: str) -> dict:
    """
    Returns the prediction as a dictionary with label and score.
    This is the main interface for the API or any client.
    """
    result = pipe(text)[0]  # HuggingFace pipeline returns a list
    return {
        "label": result["label"],
        "score": result["score"]
    }
