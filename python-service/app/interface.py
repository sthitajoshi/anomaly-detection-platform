from app.model import get_model

pipe = get_model()  # load model once at import

def predict_log(text: str, threshold: float = 0.5) -> dict:
    """
    Return model decision with label, score, and raw probabilities.
    - Uses `return_all_scores=True` to fetch probabilities for all labels.
    - Picks the Anomaly label if its probability >= threshold; otherwise top label.
    """
    # Transformers pipeline returns: [[{"label": "Normal", "score": 0.98}, {"label": "Anomaly", "score": 0.02}]]
    raw = pipe(text, return_all_scores=True)[0]
    probs = {item["label"]: float(item["score"]) for item in raw}

    # Try to locate the anomaly class in a robust, case-insensitive way
    anomaly_label = None
    anomaly_score = 0.0
    for k, v in probs.items():
        if k.lower().startswith("anomaly"):
            anomaly_label = k
            anomaly_score = v
            break

    # Decision rule: choose anomaly if available and above threshold; else choose top class
    if anomaly_label is not None and anomaly_score >= threshold:
        label = anomaly_label
        score = anomaly_score
    else:
        label = max(probs, key=probs.get)
        score = probs[label]

    return {"label": label, "score": score, "probs": probs}
