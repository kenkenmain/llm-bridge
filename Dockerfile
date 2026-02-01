# Production image: runtime base + Bazel-built Go binary.
#
# Build the binary and stage it:
#   bazel build //cmd/llm-bridge
#   cp -L bazel-bin/cmd/llm-bridge/llm-bridge_/llm-bridge .build/llm-bridge
# Then build this image:
#   docker build -t llm-bridge:latest .
FROM llm-bridge-base:latest

COPY .build/llm-bridge /usr/local/bin/llm-bridge

ENV LLM_BRIDGE_CONFIG=/etc/llm-bridge/llm-bridge.yaml

ENTRYPOINT ["llm-bridge"]
CMD ["serve", "--config", "/etc/llm-bridge/llm-bridge.yaml"]
