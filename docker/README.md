# Docker Compose Modular Setup

This directory contains modularized Docker Compose files for the homeGPT stack. Each service has its own compose file, making it easy to start individual services, add new models, or customize the deployment.

## Architecture

```
docker-compose.yml (Main orchestrator)
    │
    ├─── compose-model-manager.yml           → Model Manager (Go service)
    ├─── compose-vllm-qwen3-vl-30b-a3b.yml   → vLLM Instance 1 (Qwen)
    ├─── compose-vllm-gpt-oss-20b.yml        → vLLM Instance 2 (GPT-OSS)
    └─── compose-webui.yml                   → Open WebUI

All services connect via: homegpt-network (bridge network)
```

## File Structure

```
docker/
├── docker-compose.yml                          # Main file - includes all modules
├── compose-model-manager.yml                   # Go service (port 9000)
├── compose-vllm-qwen3-vl-30b-a3b.yml          # vLLM Qwen instance (port 8001)
├── compose-vllm-gpt-oss-20b.yml               # vLLM GPT-OSS instance (port 8002)
├── compose-webui.yml                          # Open WebUI (port 3000)
└── README.md                                  # This file
```

## Usage Patterns

### Start All Services (Production)
```bash
cd docker
docker compose up -d
```

This uses `docker-compose.yml` which includes all module files via the `include` directive.

### Start Specific Services (Development)

**Only Model Manager:**
```bash
docker compose -f compose-model-manager.yml up
```

**Test Single vLLM Server - Qwen Only (requires 2 GPUs):**
```bash
docker compose -f compose-vllm-qwen3-vl-30b-a3b.yml up -d
# Access at http://localhost:8001
# Check health: curl http://localhost:8001/health
```

**Test Single vLLM Server - GPT-OSS Only (requires 2 GPUs):**
```bash
docker compose -f compose-vllm-gpt-oss-20b.yml up -d
# Access at http://localhost:8002
# Check health: curl http://localhost:8002/health
```

**Both vLLM instances (requires 4 GPUs total):**
```bash
docker compose -f compose-vllm-qwen3-vl-30b-a3b.yml -f compose-vllm-gpt-oss-20b.yml up -d
```

**Model Manager + Single vLLM instance:**
```bash
docker compose -f compose-model-manager.yml -f compose-vllm-qwen3-vl-30b-a3b.yml up
```

**Custom combination:**
```bash
docker compose \
  -f compose-model-manager.yml \
  -f compose-vllm-qwen3-vl-30b-a3b.yml \
  -f compose-webui.yml \
  up -d
```

### Build and Start

```bash
# Build all services
docker compose build

# Build specific service
docker compose build model-manager

# Build and start
docker compose up --build
```

### Viewing Logs

```bash
# All services
docker compose logs -f

# Specific service
docker compose logs -f model-manager
docker compose logs -f vllm-qwen

# Last 100 lines
docker compose logs --tail=100 vllm-gptoss
```

### Stop and Clean Up

```bash
# Stop all services
docker compose down

# Stop and remove volumes
docker compose down -v

# Stop specific service
docker compose stop model-manager
```

## Service Details

### compose-model-manager.yml
**Purpose:** Go service that orchestrates model switching

**Key Features:**
- Builds from `../model-manager/Dockerfile`
- Mounts `config.yaml` as read-only
- Exposes port 9000 for HTTP API
- Environment variables: `CONFIG_PATH`, `PORT`

**Dependencies:** Requires vLLM instances to be running

**Network:** Connects to `homegpt-network`

### compose-vllm-qwen3-vl-30b-a3b.yml
**Purpose:** vLLM instance running Qwen3-VL-30B model

**Key Features:**
- Image: `vllm/vllm-openai:v0.11.0-x86_64`
- GPU: Requires 2 NVIDIA GPUs (tensor parallelism)
- Port: 8001 (maps to container port 8000)
- Sleep mode: Enabled via `--enable-sleep-mode`
- Model: Qwen/Qwen3-VL-30B-A3B-Instruct-FP8
- Memory: 90% GPU utilization, 128k max tokens
- Tool calling: Auto tool choice enabled (hermes parser)

**Environment:**
- `VLLM_SERVER_DEV_MODE=1` - Enables sleep endpoints
- `HUGGING_FACE_HUB_TOKEN` - For model downloads
- `CUDA_VISIBLE_DEVICES=0,1` - Use GPUs 0 and 1
- `VLLM_HOST_IP=127.0.0.1` - Host binding

**Volumes:**
- `~/.cache/huggingface` - Model cache (shared across instances)

### compose-vllm-gpt-oss-20b.yml
**Purpose:** vLLM instance running GPT-OSS-20B model

**Configuration:** 
- Model: `openai/gpt-oss-20b`
- GPU: Requires 2 NVIDIA GPUs (tensor parallelism)
- Port: 8002
- Container name: `vllm-gptoss`
- Memory: 80% GPU utilization, 128k max tokens
- Tool calling: Auto tool choice enabled (openai parser)
- Max batched tokens: 30k

### compose-webui.yml
**Purpose:** Open WebUI interface for chat

**Key Features:**
- Image: `ghcr.io/open-webui/open-webui:main`
- Port: 3000 (maps to container port 8080)
- Volume: `open-webui` - Persistent data storage

**Environment:**
- `OPENAI_API_BASE_URL=http://vllm-qwen:8000/v1` - Default model endpoint
- `MODEL_MANAGER_URL=http://model-manager:9000` - Model switcher API

**Dependencies:** 
- `vllm-qwen` (default active model)
- `model-manager` (switching service)

## Adding a New Model

### Step 1: Create Compose File

Create `compose-vllm-newmodel.yml`:

```yaml
services:
  vllm-newmodel:
    image: vllm/vllm-openai:v0.11.0-x86_64
    container_name: vllm-newmodel
    restart: unless-stopped
    command: >
      --model org/model-name
      --gpu-memory-utilization 0.85
      --max-model-len 128k
      --tensor-parallel-size 2
      --enable-sleep-mode
    ports:
      - "8003:8000"  # Use next available port (8003, 8004, etc.)
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 2
              capabilities: [gpu]
    volumes:
      - ~/.cache/huggingface:/root/.cache/huggingface
    environment:
      - HUGGING_FACE_HUB_TOKEN=${HF_TOKEN}
      - VLLM_SERVER_DEV_MODE=1
      - CUDA_VISIBLE_DEVICES=0,1
      - VLLM_HOST_IP=127.0.0.1
    ipc: host
    networks:
      - homegpt-network
```

### Step 2: Update Main Compose File

Edit `docker-compose.yml`:

```yaml
include:
  - compose-model-manager.yml
  - compose-vllm-qwen3-vl-30b-a3b.yml
  - compose-vllm-gpt-oss-20b.yml
  - compose-vllm-newmodel.yml  # ← Add this line
  - compose-webui.yml
```

### Step 3: Update Model Manager Config

Edit `../config.yaml`:

```yaml
models:
  - id: "qwen3-vl-30b"
    name: "Qwen3 VL 30B"
    endpoint: "http://vllm-qwen:8000"
    status: "active"
  
  - id: "gpt-oss-20b"
    name: "GPT-OSS 20B"
    endpoint: "http://vllm-gptoss:8000"
    status: "sleeping"
  
  - id: "new-model-id"          # ← Add new model
    name: "New Model Name"
    endpoint: "http://vllm-newmodel:8000"
    status: "sleeping"
```

### Step 4: Deploy

```bash
# Rebuild model manager (to pick up new config)
docker compose build model-manager

# Start all services
docker compose up -d

# Or start just the new model
docker compose -f compose-vllm-newmodel.yml up -d
```

## Environment Variables

Create a `.env` file in this directory:

```bash
# HuggingFace token for private models
HF_TOKEN=hf_xxxxxxxxxxxxxxxxxxxx

# Override default ports (optional)
# MODEL_MANAGER_PORT=9000
# VLLM_QWEN_PORT=8001
# VLLM_GPTOSS_PORT=8002
# WEBUI_PORT=3000
```

Load it automatically:
```bash
docker compose up  # Reads .env automatically
```

## Network Configuration

All services use the `homegpt-network` bridge network, defined in the main `docker-compose.yml`:

```yaml
networks:
  homegpt-network:
    driver: bridge
```

**Service Discovery:**
- Containers can reach each other by service name
- Example: Model Manager calls `http://vllm-qwen:8000/health`
- External access: Use host ports (9000, 8001, 8002, 3000)

**Inspect network:**
```bash
docker network inspect homegpt-network
```

## Volume Management

### Open WebUI Volume
```bash
# Inspect volume
docker volume inspect open-webui

# Backup data
docker run --rm -v open-webui:/data -v $(pwd):/backup \
  alpine tar czf /backup/openwebui-backup.tar.gz /data

# Restore data
docker run --rm -v open-webui:/data -v $(pwd):/backup \
  alpine tar xzf /backup/openwebui-backup.tar.gz -C /
```

### HuggingFace Cache
Models are cached at `~/.cache/huggingface` on the host:

```bash
# Check cache size
du -sh ~/.cache/huggingface

# Clear cache (will re-download models)
rm -rf ~/.cache/huggingface/*
```

## Troubleshooting

### Service Won't Start

```bash
# Check logs
docker compose logs <service-name>

# Check if port is in use
sudo lsof -i :9000
sudo lsof -i :8001

# Verify network exists
docker network ls | grep homegpt
```

### GPU Not Available

```bash
# Verify NVIDIA runtime
docker run --rm --gpus all nvidia/cuda:12.3.0-base-ubuntu24.04 nvidia-smi

# Check GPU allocation in container
docker compose exec vllm-qwen nvidia-smi
```

### Model Download Issues

```bash
# Check HuggingFace token
echo $HF_TOKEN

# Manually download model
huggingface-cli login
huggingface-cli download QuantTrio/Qwen3-VL-30B-A3B-Instruct-AWQ

# Verify cache
ls -la ~/.cache/huggingface/hub/
```

### Network Connectivity Issues

```bash
# Test internal connectivity from model-manager
docker compose exec model-manager wget -O- http://vllm-qwen:8000/health

# Test from host
curl http://localhost:8001/health
curl http://localhost:9000/health
```

### Rebuild After Changes

```bash
# Rebuild specific service
docker compose build model-manager

# Rebuild all and restart
docker compose up --build --force-recreate
```

## Advanced Configurations

### Override Resources

Create `docker-compose.override.yml`:

```yaml
services:
  vllm-qwen:
    deploy:
      resources:
        limits:
          memory: 64G
        reservations:
          memory: 32G
```

### Custom Commands

Override vLLM parameters:

```yaml
services:
  vllm-qwen:
    command: >
      --model Qwen/Qwen3-VL-30B-A3B-Instruct-FP8
      --gpu-memory-utilization 0.90
      --max-model-len 128k
      --max-num-batched-tokens 30k
      --tensor-parallel-size 2
      --enable-sleep-mode
      --enable-auto-tool-choice
      --tool-call-parser hermes
```

### Health Checks

Add health checks to services:

```yaml
services:
  model-manager:
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
```

## Production Deployment

### Recommended Setup

1. **Use specific image tags** (not `latest` or `main`):
```yaml
image: vllm/vllm-openai:v0.11.0-x86_64  # Already versioned ✓
image: ghcr.io/open-webui/open-webui:v0.1.122  # Pin version
```

2. **Set resource limits:**
```yaml
deploy:
  resources:
    limits:
      memory: 64G
      cpus: '8'
```

3. **Configure restart policies:**
```yaml
restart: unless-stopped  # Already configured ✓
```

4. **Enable logging:**
```yaml
logging:
  driver: "json-file"
  options:
    max-size: "10m"
    max-file: "3"
```

### Monitoring

```bash
# Check resource usage
docker stats

# Monitor logs in real-time
docker compose logs -f --tail=100

# Check service health
docker compose ps
```

## References

- [Docker Compose Include Directive](https://docs.docker.com/compose/multiple-compose-files/include/)
- [vLLM Docker Guide](https://docs.vllm.ai/en/latest/deployment/docker.html)
- [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/overview.html)
- [Open WebUI Environment Variables](https://github.com/open-webui/open-webui#environment-variables)

