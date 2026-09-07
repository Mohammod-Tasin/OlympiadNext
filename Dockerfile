# ---------- Stage 1: build ----------
FROM golang:1.26-alpine AS build
WORKDIR /app

# Copy dependency files first so Docker can cache this layer
# (only re-downloads dependencies if go.mod/go.sum actually change)
COPY go.mod go.sum ./
RUN go mod download

# Now copy the rest of the source code
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o shikhor .

# ---------- Stage 2: run ----------
FROM alpine:latest
WORKDIR /app

# ca-certificates is required for outbound HTTPS calls —
# specifically Google's idtoken.Validate() for Google Sign-In
RUN apk add --no-cache ca-certificates

COPY --from=build /app/shikhor .

EXPOSE 8080
ENTRYPOINT ["./shikhor"]