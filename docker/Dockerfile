FROM scratch

ARG TARGETARCH

COPY --chown=nobody:nogroup artifacts/promptpipe-${TARGETARCH} /opt/promptpipe/promptpipe

ENV PATH="/opt/promptpipe:${PATH}"

ENTRYPOINT ["promptpipe"]
