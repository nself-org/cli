package scaffold

// service_write.go — writing the scaffolded custom service files.
//
// Purpose: render and write the full set of CS_N custom service files to disk, used by Run in service.go, split out for file size.
// Inputs: the resolved Options and slot assignment from service.go.
// Outputs: the scaffolded custom service files written under the project's custom service directory.
// Constraints: pure move from service.go (CLI-R12 Batch E); no behaviour change.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// writeScaffold creates the starter files for the custom service.
func writeScaffold(dir, name, lang string, port int, imageRef func(string, string) string) ([]string, error) {
	if imageRef == nil {
		return nil, fmt.Errorf("image resolver is required to scaffold service files")
	}
	type fileSpec struct {
		rel     string
		content string
		mode    os.FileMode
	}

	upperName := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))

	var specs []fileSpec

	readme := fmt.Sprintf("# %s\n\nCustom nSelf service scaffolded via `nself service add`.\n\n## Start\n\n```bash\nnself start\n```\n\n## Port\n\n%d (auto-assigned CS_%s)\n", name, port, upperName)
	specs = append(specs, fileSpec{"README.md", readme, 0644})
	specs = append(specs, fileSpec{".dockerignore", ".git\ntmp/\n*.test\n", 0644})

	switch lang {
	case "go":
		specs = append(specs,
			fileSpec{"go.mod", fmt.Sprintf("module %s\n\ngo 1.23.0\n", name), 0644},
			fileSpec{"main.go", fmt.Sprintf(`package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %%v\n", err)
		return
	}
}

func run() error {
	port := os.Getenv("%s_PORT")
	if port == "" {
		port = "%d"
	}
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	fmt.Println("%s listening on :" + port)
	return http.ListenAndServe(":"+port, nil)
}
`, upperName, port, name), 0644},
			fileSpec{"Dockerfile", fmt.Sprintf(`FROM %s AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/%s .

FROM %s
COPY --from=build /out/%s /%s
EXPOSE %d
ENTRYPOINT ["/%s"]
`, imageRef("golang", ""), name, imageRef("distroless-static", ""), name, name, port, name), 0644},
		)
	case "node":
		specs = append(specs,
			fileSpec{"package.json", fmt.Sprintf(`{
  "name": "%s",
  "version": "0.1.0",
  "scripts": { "start": "tsx src/index.ts", "dev": "tsx watch src/index.ts" },
  "license": "MIT"
}
`, name), 0644},
			fileSpec{"tsconfig.json", `{"compilerOptions":{"target":"ES2022","module":"NodeNext","moduleResolution":"NodeNext","strict":true}}`, 0644},
			fileSpec{"src/index.ts", fmt.Sprintf(`import { createServer } from "http";
const port = process.env.%s_PORT ?? "%d";
createServer((req, res) => {
  if (req.url === "/healthz") { res.writeHead(200); res.end("ok"); return; }
  res.writeHead(404); res.end("not found");
}).listen(port, () => console.log("%s listening on :" + port));
`, upperName, port, name), 0644},
			fileSpec{"Dockerfile", fmt.Sprintf(`FROM %s
WORKDIR /app
COPY package.json ./
RUN npm install
COPY . .
EXPOSE %d
CMD ["node", "-e", "require('./src/index.ts')"]
`, imageRef("node", ""), port), 0644},
		)
	case "python":
		specs = append(specs,
			fileSpec{"requirements.txt", "fastapi>=0.110\nuvicorn>=0.29\n", 0644},
			fileSpec{"main.py", fmt.Sprintf(`import os
from fastapi import FastAPI
import uvicorn

app = FastAPI()

@app.get("/healthz")
def healthz():
    return {"status": "ok"}

if __name__ == "__main__":
    port = int(os.environ.get("%s_PORT", %d))
    uvicorn.run(app, host="0.0.0.0", port=port)
`, upperName, port), 0644},
			fileSpec{"Dockerfile", fmt.Sprintf(`FROM %s
WORKDIR /app
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
EXPOSE %d
CMD ["python", "main.py"]
`, imageRef("python", ""), port), 0644},
		)
	case "static":
		specs = append(specs,
			fileSpec{"index.html", fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>%s</title></head>
<body><h1>%s</h1><p>Served by nSelf custom service on port %d.</p></body>
</html>
`, name, name, port), 0644},
			fileSpec{"nginx.conf", fmt.Sprintf(`# nSelf custom service — static nginx snippet
# Include this from your nginx conf.d/ directory.
server {
    listen %d;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;
    location / { try_files $uri $uri/ =404; }
}
`, port), 0644},
			fileSpec{"Dockerfile", fmt.Sprintf(`FROM %s
COPY index.html /usr/share/nginx/html/index.html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE %d
CMD ["nginx", "-g", "daemon off;"]
`, imageRef("nginx", ""), port), 0644},
		)
	case "rust":
		specs = append(specs,
			fileSpec{"Cargo.toml", fmt.Sprintf(`[package]
name = "%s"
version = "0.1.0"
edition = "2021"

[dependencies]
actix-web = "4"
tokio = { version = "1", features = ["full"] }
`, name), 0644},
			fileSpec{"src/main.rs", fmt.Sprintf(`use actix_web::{web, App, HttpServer, HttpResponse};

async fn healthz() -> HttpResponse { HttpResponse::Ok().body("ok") }

#[actix_web::main]
async fn main() -> std::io::Result<()> {
    let port = std::env::var("%s_PORT").unwrap_or_else(|_| "%d".to_string());
    println!("%s listening on :{}", port);
    HttpServer::new(|| App::new().route("/healthz", web::get().to(healthz)))
        .bind(format!("0.0.0.0:{}", port))?
        .run().await
}
`, upperName, port, name), 0644},
			fileSpec{"Dockerfile", fmt.Sprintf(`FROM %s AS builder
WORKDIR /app
RUN apk add --no-cache musl-dev
COPY Cargo.toml ./
COPY src/ src/
RUN cargo build --release
FROM %s
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/target/release/%s /usr/local/bin/service
EXPOSE %d
CMD ["service"]
`, imageRef("rust", ""), imageRef("alpine", ""), name, port), 0644},
		)
	default: // "other"
		specs = append(specs,
			fileSpec{"main.sh", fmt.Sprintf(`#!/bin/sh
PORT="${%s_PORT:-%d}"
echo "%s listening on $PORT"
# Replace with your service entrypoint.
exec nc -l -p "$PORT"
`, upperName, port, name), 0750},
			fileSpec{"Dockerfile", fmt.Sprintf(`FROM %s
COPY main.sh /main.sh
RUN chmod +x /main.sh
EXPOSE %d
CMD ["/main.sh"]
`, imageRef("alpine", ""), port), 0644},
		)
	}

	var written []string
	for _, spec := range specs {
		fullPath := filepath.Join(dir, spec.rel)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return nil, fmt.Errorf("creating directory for %s: %w", spec.rel, err)
		}
		if err := os.WriteFile(fullPath, []byte(spec.content), spec.mode); err != nil {
			return nil, fmt.Errorf("writing %s: %w", spec.rel, err)
		}
		written = append(written, spec.rel)
	}
	return written, nil
}
