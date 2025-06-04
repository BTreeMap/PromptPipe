FROM scratch

ARG TARGETARCH

COPY artifacts/promptpipe-${TARGETARCH} /opt/promptpipe/promptpipe

ENV PATH="/opt/promptpipe:${PATH}"

ENTRYPOINT ["promptpipe"]
