#####################################################
### Copy platform specific binary
FROM bash AS copy-binary
ARG TARGETPLATFORM

RUN echo "Target Platform = ${TARGETPLATFORM}"

COPY dist .
RUN if [ "$TARGETPLATFORM" = "linux/amd64" ];  then cp torrentindexer_linux_amd64_linux_amd64_v1/torrent-indexer /torrent-indexer; fi
RUN if [ "$TARGETPLATFORM" = "linux/386" ];  then cp torrentindexer_linux_386_linux_386_sse2/torrent-indexer /torrent-indexer; fi
RUN if [ "$TARGETPLATFORM" = "linux/arm64" ];  then cp torrentindexer_linux_arm64_linux_arm64_v8.0/torrent-indexer /torrent-indexer; fi
RUN if [ "$TARGETPLATFORM" = "linux/arm/v6" ]; then cp torrentindexer_linux_arm_linux_arm_6/torrent-indexer /torrent-indexer; fi
RUN if [ "$TARGETPLATFORM" = "linux/arm/v7" ]; then cp torrentindexer_linux_arm_linux_arm_7/torrent-indexer /torrent-indexer; fi
RUN if [ "$TARGETPLATFORM" = "linux/riscv64" ]; then cp torrentindexer_linux_riscv64_linux_riscv64_rva20u64/torrent-indexer /torrent-indexer; fi
RUN chmod +x /torrent-indexer


#####################################################
### Build Final Image
FROM alpine AS release
LABEL maintainer="felipevm97@gmail.com"

COPY --from=copy-binary /torrent-indexer /app/

WORKDIR /app

ENTRYPOINT ["/app/torrent-indexer"]