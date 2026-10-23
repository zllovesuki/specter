FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install --yes --no-install-recommends xorriso \
    && rm -rf /var/lib/apt/lists/*

ENTRYPOINT ["xorriso"]
