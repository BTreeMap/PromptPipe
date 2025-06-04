# Use a minimal base image
ARG PLATFORM
FROM --platform=$PLATFORM scratch AS builder

# Build stage for static Go binary
COPY . .

# Set source date epoch for reproducible builds
ARG SOURCE_DATE_EPOCH
ENV SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH}

RUN \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=${PLATFORM} \
    go build -ldflags "-s -w -buildid= " -o promptpipe ./cmd/PromptPipe

# Final stage
FROM scratch

# Copy binary
COPY --from=builder /promptpipe /promptpipe

# Set entrypoint
ENTRYPOINT ["/promptpipe"]
