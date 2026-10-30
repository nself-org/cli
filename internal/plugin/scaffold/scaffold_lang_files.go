package scaffold

// scaffold_lang_files.go — per-language file sets and the Dockerfile.
//
// Purpose: assemble the Go, Rust, Node and static-asset file sets, and build the plugin's Dockerfile, used by Run in scaffold.go, split out for file size.
// Inputs: the resolved Params/Options for the plugin being scaffolded.
// Outputs: language-specific files (path + rendered content) and a Dockerfile.
// Constraints: pure move from scaffold.go (CLI-R12 Batch F); no behaviour change. Template strings stay in scaffold_templates_infra.go / scaffold_templates_code.go per the existing decomposition.

func addGoFiles(files map[string]string, p Params) error {
	gomod, err := render(`module github.com/nself-org/{{.RepoBucket}}/{{.Name}}

go 1.23.0

require github.com/nself-org/plugin-sdk-go v0.1.0
`, p)
	if err != nil {
		return err
	}
	files["go.mod"] = gomod
	files["go.sum"] = ""
	main, err := render(tmplMain, p)
	if err != nil {
		return err
	}
	files["cmd/main.go"] = main
	config, err := render(tmplConfig, p)
	if err != nil {
		return err
	}
	files["internal/config/config.go"] = config
	server, err := render(tmplServer, p)
	if err != nil {
		return err
	}
	files["internal/server/server.go"] = server
	serverTest, err := render(tmplServerTest, p)
	if err != nil {
		return err
	}
	files["internal/server/server_test.go"] = serverTest
	return nil
}

func addRustFiles(files map[string]string, p Params) error {
	cargoToml, err := render(`[package]
name = "{{.Name}}"
version = "0.1.0"
edition = "2021"

[dependencies]
actix-web = "4"
tokio = { version = "1", features = ["full"] }
`, p)
	if err != nil {
		return err
	}
	files["Cargo.toml"] = cargoToml
	files["Cargo.lock"] = ""
	mainRs, err := render(`use actix_web::{web, App, HttpServer, HttpResponse};

async fn healthz() -> HttpResponse {
    HttpResponse::Ok().body("ok")
}

#[actix_web::main]
async fn main() -> std::io::Result<()> {
    println!("{{.Name}} plugin starting on :{{.Port}}");
    HttpServer::new(|| {
        App::new().route("/healthz", web::get().to(healthz))
    })
    .bind("0.0.0.0:{{.Port}}")?
    .run()
    .await
}
`, p)
	if err != nil {
		return err
	}
	files["src/main.rs"] = mainRs
	return nil
}

func addNodeFiles(files map[string]string, p Params) error {
	packageJSON, err := render(`{
  "name": "nself-{{.Name}}",
  "version": "0.1.0",
  "description": "{{.Description}}",
  "main": "dist/index.js",
  "scripts": {
    "build": "tsc",
    "dev": "tsx watch src/index.ts",
    "start": "node dist/index.js"
  },
  "license": "{{.License}}"
}
`, p)
	if err != nil {
		return err
	}
	files["package.json"] = packageJSON
	files["tsconfig.json"] = `{
  "compilerOptions": {
    "target": "ES2022",
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "outDir": "dist",
    "rootDir": "src",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true
  },
  "include": ["src"]
}
`
	indexTs, err := render(`import { createServer } from "http";

const server = createServer((req, res) => {
  if (req.url === "/healthz") {
    res.writeHead(200);
    res.end("ok");
    return;
  }
  res.writeHead(404);
  res.end("not found");
});

server.listen({{.Port}}, () => {
  console.log("{{.Name}} plugin listening on :{{.Port}}");
});
`, p)
	if err != nil {
		return err
	}
	files["src/index.ts"] = indexTs
	return nil
}

func addStaticFiles(files map[string]string, p Params) error {
	indexHTML, err := render(`<!DOCTYPE html>
<html>
<head><title>{{.Name}}</title></head>
<body><h1>{{.Name}} plugin</h1></body>
</html>
`, p)
	if err != nil {
		return err
	}
	files["static/index.html"] = indexHTML
	return nil
}

func buildDockerfile(p Params) (string, error) {
	switch p.Language {
	case "rust":
		return render(`FROM {{image "rust"}} AS builder
WORKDIR /app
RUN apk add --no-cache musl-dev
COPY Cargo.toml Cargo.lock ./
COPY src/ src/
RUN cargo build --release

FROM {{image "alpine"}}
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/target/release/{{.Name}} /usr/local/bin/plugin
EXPOSE {{.Port}}
CMD ["plugin"]
`, p)
	case "node":
		return render(`FROM {{image "node"}} AS builder
WORKDIR /app
COPY package.json pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY . .
RUN pnpm build

FROM {{image "node"}}
WORKDIR /app
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/package.json .
EXPOSE {{.Port}}
CMD ["node", "dist/index.js"]
`, p)
	case "static":
		return render(`FROM {{image "nginx"}}
COPY static/ /usr/share/nginx/html/
EXPOSE {{.Port}}
CMD ["nginx", "-g", "daemon off;"]
`, p)
	default: // go
		return render(`# syntax=docker/dockerfile:1.7
FROM {{image "golang"}} AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG VERSION=0.1.0
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.Version=${VERSION}" -o /out/{{.Name}} ./cmd

FROM {{image "distroless-static"}}
USER nonroot:nonroot
COPY --from=build /out/{{.Name}} /{{.Name}}
EXPOSE {{.Port}}
ENTRYPOINT ["/{{.Name}}"]
`, p)
	}
}
