#!/usr/bin/env python3
"""Upload do CSV de teste via API. Faz login, sobe o arquivo e mostra progresso."""
import json
import sys
import time
import urllib.request
import urllib.error
from pathlib import Path

API = "http://localhost:8087"
EMAIL = "claudio_bezerra@hotmail.com"
PASSWORD = "123456"
CSV = Path(__file__).parent / "vendas-teste.csv"

def http(method, url, data=None, headers=None, files=None):
    h = dict(headers or {})
    body = None
    if files:
        # multipart manual
        boundary = "----FBFarolTest" + str(int(time.time()))
        h["Content-Type"] = f"multipart/form-data; boundary={boundary}"
        parts = []
        for name, (fname, content, ctype) in files.items():
            parts.append(f"--{boundary}\r\n".encode())
            parts.append(f'Content-Disposition: form-data; name="{name}"; filename="{fname}"\r\n'.encode())
            parts.append(f"Content-Type: {ctype}\r\n\r\n".encode())
            parts.append(content)
            parts.append(b"\r\n")
        parts.append(f"--{boundary}--\r\n".encode())
        body = b"".join(parts)
    elif data is not None:
        body = json.dumps(data).encode()
        h["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=body, headers=h, method=method)
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            return resp.status, resp.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()

def main():
    if not CSV.exists():
        print(f"❌ CSV não encontrado: {CSV}")
        sys.exit(1)
    print(f"📄 CSV: {CSV} ({CSV.stat().st_size} bytes)")

    print(f"🔐 Login {EMAIL}…")
    status, body = http("POST", f"{API}/api/auth/login", data={"email": EMAIL, "password": PASSWORD})
    if status != 200:
        print(f"❌ Login falhou ({status}): {body!r}")
        sys.exit(1)
    auth = json.loads(body)
    token = auth["token"]
    company_id = auth.get("company_id") or ""
    print(f"   token ok, company={company_id[:8]}…")

    print(f"📤 Upload do CSV…")
    csv_bytes = CSV.read_bytes()
    url = f"{API}/api/v2/vendas/import?tipo_base=ATUAL&ano=2026&mes=5"
    headers = {"Authorization": f"Bearer {token}"}
    if company_id:
        headers["X-Company-ID"] = company_id
    status, body = http("POST", url, files={"file": ("vendas-teste.csv", csv_bytes, "text/csv")}, headers=headers)
    if status != 200:
        print(f"❌ Upload falhou ({status}): {body!r}")
        sys.exit(1)
    resp = json.loads(body)
    job_id = resp.get("job_id")
    print(f"   job_id = {job_id}")

    print(f"⏳ Aguardando processamento…")
    while True:
        status, body = http("GET", f"{API}/api/v2/vendas/job/{job_id}", headers=headers)
        if status != 200:
            print(f"❌ Status falhou ({status}): {body!r}")
            sys.exit(1)
        job = json.loads(body)
        st = job.get("status", "?")
        pg = job.get("progress", 0)
        msg = job.get("message", "")
        imp = job.get("importados", 0)
        print(f"   {st} {pg}% imp={imp} {msg}", flush=True)
        if st in ("done", "error", "cancelled"):
            break
        time.sleep(1)

    if st == "done":
        print(f"\n✅ Import concluído — {imp} linhas processadas")
        print(f"\nDica: acesse http://localhost:3087 e veja o Painel Vendas.")
    else:
        print(f"\n❌ Falhou: {msg}")
        sys.exit(1)

if __name__ == "__main__":
    main()
