from app.model import get_model

pipe = get_model()  # load model once at import
DEBUG_PRINTED = False  # one-time debug guard

def predict_log(text: str, threshold: float = 0.5) -> dict:
    """
    Return model decision with label, score, and raw probabilities.
    - Uses `return_all_scores=True` to fetch probabilities for all labels.
    - Picks the Anomaly label if its probability >= threshold; otherwise top label.
    - Adds debug safety in case 'Anomaly' is missing or mislabeled.
    """
    # Get raw predictions from the pipeline
    raw = pipe(text, return_all_scores=True)[0]
    # One-time debug print to confirm actual model labels
    global DEBUG_PRINTED
    if not DEBUG_PRINTED:
        print("DEBUG raw model output:", raw, flush=True)
        DEBUG_PRINTED = True

    # Convert to dict of label -> score
    probs = {item["label"]: float(item["score"]) for item in raw}

    # Normalize label names to lower-case for matching
    anomaly_candidates = [k for k in probs if k.lower().startswith("anomaly")]

    # Default
    label, score = None, None

    if anomaly_candidates:
        anomaly_label = anomaly_candidates[0]
        anomaly_score = probs[anomaly_label]

        # Decision rule
        if anomaly_score >= threshold:
            label, score = anomaly_label, anomaly_score

    # Fallback: pick top label if anomaly not chosen
    if label is None:
        label = max(probs, key=probs.get)
        score = probs[label]

    return {
        "label": label,
        "score": score,
        "probs": probs,
        "raw": raw  # <-- keep raw for debugging
    }
