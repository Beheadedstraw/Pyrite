from fastapi import FastAPI, Request, Response


app = FastAPI()


CORS_HEADERS = {
    "Access-Control-Allow-Origin": "https://app.example",
    "Access-Control-Allow-Methods": "GET, OPTIONS",
    "Access-Control-Allow-Headers": "authorization,content-type",
    "Access-Control-Max-Age": "600",
    "Vary": "Origin",
}


@app.options("/{path:path}")
async def cors_preflight(path: str, request: Request):
    origin = request.headers.get("origin", "")
    method = request.headers.get("access-control-request-method", "")
    headers = request.headers.get("access-control-request-headers", "")
    allowed = (
        origin == "https://app.example"
        and method == "GET"
        and "authorization" in headers
        and "content-type" in headers
    )
    if allowed:
        return Response("", status_code=204, headers=CORS_HEADERS)
    return Response("cors rejected\n", status_code=403, media_type="text/plain")


@app.get("/api/ping")
async def ping():
    return {"ok": True, "service": "fastapi"}


@app.get("/api/score")
async def score():
    return {"name": "Ada", "score": 42}


@app.get("/api/text")
async def text():
    return Response("hello from FastAPI\n", media_type="text/plain")


@app.get("/api/user")
async def user():
    return {
        "id": 42,
        "name": "Ada Lovelace",
        "language": "FastAPI",
        "active": True,
        "roles": ["admin", "bench"],
    }


@app.get("/api/items")
async def items():
    return [
        {"id": 1, "name": "one"},
        {"id": 2, "name": "two"},
        {"id": 3, "name": "three"},
        {"id": 4, "name": "four"},
    ]


@app.get("/api/compute")
async def compute():
    total = 0
    for i in range(500):
        total += i * 3
    return Response(f"total={total}\n", media_type="text/plain")


@app.get("/api/auth")
async def auth():
    score = 0
    host_seen = 1
    keep_seen = 1
    for i in range(120):
        score += (i * 17) + host_seen + keep_seen
    body = f"authenticated=true\nclaims=3\nscore={score}\n"
    return Response(body, media_type="text/plain")


@app.get("/api/search")
async def search(q: str = "ada"):
    query = q.strip().lower()
    hits = 0
    score = 0
    for i in range(80):
        if "ada" in query:
            hits += 1
            score += i + 7
        if "hopper" in query:
            hits += 1
            score += i + 11
        if "math" in query:
            hits += 2
            score += i + 17
    body = f"query={query}\nhits={hits}\nscore={score}\n"
    return Response(body, media_type="text/plain")


@app.get("/api/report")
async def report():
    revenue = 0
    cost = 0
    for day in range(1, 91):
        revenue += day * 37
        cost += day * 19 + day
    margin = revenue - cost
    body = f"quarter=Q1\nrevenue={revenue}\ncost={cost}\nmargin={margin}\n"
    return Response(body, media_type="text/plain")


@app.get("/api/profile")
async def profile():
    points = 0
    streak = 0
    for i in range(240):
        points += i * 9
        if i < 50:
            streak += 1
    body = f"id=42\nname=Ada Lovelace\nplan=pro\nregion=us-central\npoints={points}\nstreak={streak}\n"
    return Response(body, media_type="text/plain")


@app.get("/api/transform")
async def transform():
    text = "  Ada,Lovelace,Compiler,Math,Notes  "
    total = 0
    current = text
    for i in range(180):
        current = current.strip().upper().replace(",", "|").replace("NOTES", "REPORT")
        if "COMPILER" in current:
            total += len(current) + i
    body = f"text={current}\ntotal={total}\n"
    return Response(body, media_type="text/plain")


@app.get("/api/dashboard")
async def dashboard():
    active = 0
    errors = 0
    latency = 0
    for i in range(300):
        active += i * 3
        if i < 9:
            errors += 1
        latency += i * i
    body = f"service=pyrite-bench\nwindow=5m\nactive={active}\nerrors={errors}\nlatency={latency}\nstatus=ok\n"
    return Response(body, media_type="text/plain")
