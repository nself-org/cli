FROM alpine:latest

LABEL maintainer="acamarata <contact@acamarata.com>"
LABEL version="0.9.9"
LABEL description="Self-hosted infrastructure manager for developers"

# Install dependencies
RUN apk add --no-cache \
    bash \
    curl \
    git \
    docker-cli \
    docker-compose \
    openssl \
    ca-certificates

# Create nself user
RUN addgroup -g 1000 nself && \
    adduser -D -s /bin/bash -G nself -u 1000 nself

# Copy only essential nself files (exclude tests, docs, examples)
COPY bin/ /opt/nself/bin/
COPY src/VERSION /opt/nself/src/VERSION
COPY src/cli/ /opt/nself/src/cli/
COPY src/lib/ /opt/nself/src/lib/
COPY src/templates/ /opt/nself/src/templates/
COPY src/services/ /opt/nself/src/services/
COPY LICENSE README.md /opt/nself/

RUN chmod +x /opt/nself/bin/nself && \
    ln -s /opt/nself/bin/nself /usr/local/bin/nself

# Set up workspace
WORKDIR /workspace
RUN chown nself:nself /workspace

# Switch to nself user
USER nself

ENTRYPOINT ["nself"]
CMD ["help"]