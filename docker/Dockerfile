FROM scratch

ARG TARGETARCH

COPY --chmod=755 artifacts/promptpipe-${TARGETARCH} /opt/promptpipe/promptpipe

ENV PATH="/opt/promptpipe"

ENTRYPOINT ["promptpipe"]
