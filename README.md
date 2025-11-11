# homeGPT - Hot-Swappable vLLM Model Manager

A production-ready system for running multiple vLLM models with hot-swapping capability using vLLM's sleep mode feature. This enables efficient GPU memory management by putting inactive models to sleep (offloading to CPU RAM or discarding weights) while keeping the active model on GPU.

## Quick Start

```bash
# 1. Install dependencies
sudo apt install yq  # YAML parser for bootstrap script

# 2. Run bootstrap script (handles proper startup sequence)
./bootstrap.sh

# This will:
# - Load and cache all models (one at a time to avoid OOM)
# - Put all models to sleep
# - Wake only the default model
# - Start Model Manager and WebUI
```

Access points:
- Model Manager API: http://localhost:9000
- Open WebUI: http://localhost:3000
- vLLM Qwen: http://localhost:8001
- vLLM GPT-OSS: http://localhost:8002

## Test Model Switching

```bash
# Check current status
curl -s http://localhost:9000/models | jq

# Switch to GPT-OSS
curl -X POST http://localhost:9000/switch \
  -H "Content-Type: application/json" \
  -d '{"model_id": "gpt-oss-20b"}' | jq

# Switch back to Qwen
curl -X POST http://localhost:9000/switch \
  -H "Content-Type: application/json" \
  -d '{"model_id": "qwen3-vl-30b"}' | jq
```

## Architecture Overview

```
┌─────────────────┐
│   Open WebUI    │ (Port 3000)
└────────┬────────┘
         │
         ▼
┌─────────────────┐     ┌──────────────────┐
│ Model Manager   │────▶│  vLLM Instance 1 │ (Port 8001 - Qwen)
│   (Go Service)  │     │  [Active/Sleep]  │
│   Port 9000     │     └──────────────────┘
└─────────────────┘     ┌──────────────────┐
                        │  vLLM Instance 2 │ (Port 8002 - GPT-OSS)
                        │  [Active/Sleep]  │
                        └──────────────────┘
```

## Project Structure

```
homeGPT/
├── model-manager/          # Go service for model switching
│   ├── cmd/switcher/       # Main application entry point
│   ├── internal/           # Internal packages
│   │   ├── config/         # Configuration loading
│   │   ├── handlers/       # HTTP handlers (Gin)
│   │   ├── switcher/       # Core switching logic
│   │   └── vllm/           # vLLM HTTP client
│   ├── pkg/models/         # Shared data structures
│   ├── config.yaml         # Model definitions & settings
│   ├── Dockerfile          # Multi-stage build
│   └── go.mod              # Go dependencies
├── docker/                 # Modular Docker Compose files
│   ├── docker-compose.yml  # Main orchestration (includes all)
│   ├── compose-model-manager.yml
│   ├── compose-vllm-qwen.yml
│   ├── compose-vllm-gptoss.yml
│   └── compose-webui.yml
└── test-switcher.sh        # API test script
```

## Components

### Model Manager (Go Service)
- **Port:** 9000
- **Language:** Go 1.21+
- **Framework:** Gin web framework
- **Function:** Orchestrates model switching by calling vLLM sleep/wake endpoints
- **Features:**
  - Thread-safe concurrent request handling (sync.RWMutex)
  - Memory-aware sleep level selection (Level 1 vs Level 2)
  - Health monitoring with configurable retry logic
  - RESTful HTTP API

### Model Manager API Endpoints
- `GET /health` - Service health check
- `GET /models` - List all models with current status (active/sleeping/switching/error)
- `POST /switch` - Switch to a different model
  ```json
  {
    "model_id": "qwen3-vl-30b"
  }
  ```

### vLLM Sleep Mode
vLLM v0.11.0+ supports two sleep levels:
- **Level 1:** Offload model weights to CPU RAM (fast wake-up, requires RAM)
- **Level 2:** Discard model weights entirely (slow wake-up, no RAM needed)

The model manager automatically selects the appropriate level based on available RAM (configured in `model-manager/config.yaml`).

**IMPORTANT: Development Mode Requirements**
Sleep mode endpoints are **ONLY available when running vLLM in development mode**:
- Environment variable: `VLLM_SERVER_DEV_MODE=1` ✅ (configured in Docker Compose)
- Server flag: `--enable-sleep-mode` ✅ (configured in Docker Compose)

These endpoints should **NOT be exposed to end users in production** according to vLLM documentation.

### Sleep Mode Endpoints (vLLM)
Only available with `VLLM_SERVER_DEV_MODE=1` and `--enable-sleep-mode`:
- `POST /sleep?level=1` or `POST /sleep?level=2` - Put model to sleep
- `POST /wake_up` - Wake up sleeping model
- `GET /is_sleeping` - Check if model is currently sleeping
- `GET /is_sleeping` - Check if model is sleeping

### Sleep Mode Endpoints (vLLM)
- `POST /sleep?level=1` or `POST /sleep?level=2` - Put model to sleep
- `POST /wake_up` - Wake up sleeping model
- `GET /is_sleeping` - Check if model is sleeping

## Adding a New Model

### 1. Update Model Configuration
Edit `model-manager/config.yaml`:

```yaml
models:
  - id: "new-model-id"
    name: "New Model Name"
    endpoint: "http://vllm-newmodel:8000"
    status: "sleeping"  # or "active" for default
```

### 2. Create Docker Compose File
Create `docker/compose-vllm-newmodel.yml`:

```yaml
services:
  vllm-newmodel:
    image: vllm/vllm-openai:v0.11.0-x86_64
    container_name: vllm-newmodel
    restart: unless-stopped
    command: >
      --model org/model-name
      --gpu-memory-utilization 0.95
      --max-model-len 100000
      --enable-sleep-mode
    ports:
      - "8003:8000"  # Use next available port
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
    volumes:
      - ~/.cache/huggingface:/root/.cache/huggingface
    environment:
      - HUGGING_FACE_HUB_TOKEN=${HF_TOKEN}
      - VLLM_SERVER_DEV_MODE=1
    ipc: host
    networks:
      - homegpt-network
```

### 3. Include in Main Compose
Edit `docker/docker-compose.yml`:

```yaml
include:
  - compose-model-manager.yml
  - compose-vllm-qwen.yml
  - compose-vllm-gptoss.yml
  - compose-vllm-newmodel.yml  # Add this line
  - compose-webui.yml
```

### 4. Rebuild and Restart
```bash
cd docker
docker compose build model-manager
docker compose up -d
```

## Development Guide

### Project Layout

**Go Code Organization:**
- `cmd/switcher/main.go` - Application entry point, initializes Gin router
- `internal/config/` - Loads `config.yaml` into Go structs
- `internal/vllm/client.go` - HTTP client for vLLM API (Sleep, WakeUp, Health, IsSleeping)
- `internal/switcher/switcher.go` - **Core logic:** SwitchModel orchestrates sleep→wake→update flow
- `internal/handlers/` - Gin HTTP handlers wrapping switcher methods
- `pkg/models/` - Shared data structures (Model, Config, StatusEnum)

**Key Implementation Details:**
- Thread safety via `sync.RWMutex` in `switcher.go`
- Sleep level selection: Checks `available_ram_gb` config to decide Level 1 vs Level 2
- Health check retry logic: Configurable `max_retries` and `health_check_interval_seconds`
- Error handling: Returns errors up the stack, handlers convert to HTTP status codes

### Building the Model Manager

```bash
cd model-manager
go mod download
go build -o switcher ./cmd/switcher
./switcher  # Runs on port 9000
```

Or with Docker:
```bash
cd docker
docker compose build model-manager
```

### Testing

```bash
# Start services
cd docker
docker compose up -d

# Run tests
cd ..
./test-switcher.sh

# Manual testing
curl http://localhost:9000/health
curl http://localhost:9000/models
curl -X POST http://localhost:9000/switch \
  -H "Content-Type: application/json" \
  -d '{"model_id":"gpt-oss-20b"}'
```

### Modifying Switching Logic

The core switching algorithm is in `internal/switcher/switcher.go`:

```go
func (s *Switcher) SwitchModel(ctx context.Context, targetModelID string) error {
    // 1. Validate target model exists
    // 2. Lock for exclusive access (mutex)
    // 3. Find currently active model
    // 4. Sleep active model (determine level based on RAM)
    // 5. Wake up target model
    // 6. Wait for health check to pass
    // 7. Update model statuses
    // 8. Return success/error
}
```

To modify behavior:
- **Change sleep level logic:** Edit `determineSleepLevel()` method
- **Adjust retry behavior:** Edit `max_retries` in `config.yaml`
- **Add new endpoints:** Add methods to `internal/handlers/handlers.go`

## Configuration Reference

### model-manager/config.yaml

```yaml
models:
  - id: "qwen3-vl-30b"          # Unique identifier
    name: "Qwen3 VL 30B"        # Display name
    endpoint: "http://vllm-qwen:8000"  # vLLM service URL
    status: "active"            # Initial status: active/sleeping

switching:
  available_ram_gb: 128.0       # Total RAM for sleep level decision
  max_retries: 30               # Health check retries
  health_check_interval_seconds: 2  # Seconds between retries
```

**Sleep Level Selection:**
- If `available_ram_gb >= 64`: Use Level 1 (offload to CPU RAM)
- Otherwise: Use Level 2 (discard weights)

## System Requirements

- **OS:** Linux (Ubuntu 24.04 LTS recommended)
- **RAM:** 64GB+ (128GB recommended for Level 1 sleep)
- **GPU:** NVIDIA GPU with 24GB+ VRAM (RTX 4090, 5090, A100, etc.)
- **Storage:** 100GB+ free space for model caching
- **Docker:** 23.0+
- **NVIDIA Container Toolkit:** Latest version
- **Go:** 1.21+ (for development)

## Troubleshooting

### Model Manager Won't Start
```bash
# Check logs
docker compose logs model-manager

# Common issues:
# - config.yaml syntax error
# - Port 9000 already in use
# - Network not created
```

### Model Switch Fails
```bash
# Check vLLM instance logs
docker compose logs vllm-qwen
docker compose logs vllm-gptoss

# Verify sleep mode is enabled
curl http://localhost:8001/is_sleeping
curl http://localhost:8002/is_sleeping

# Ensure VLLM_SERVER_DEV_MODE=1 is set
```

### Model Not Downloaded
```bash
# Pre-download models
huggingface-cli download QuantTrio/Qwen3-VL-30B-A3B-Instruct-AWQ
huggingface-cli download unsloth/gpt-oss-20b

# Or set HF_TOKEN in .env file and let vLLM download on first run
```

### Out of Memory
- Reduce `--gpu-memory-utilization` from 0.95 to 0.85
- Reduce `--max-model-len` to limit context window
- Use smaller quantized models (AWQ, GPTQ)
- Ensure sufficient RAM for Level 1 sleep (128GB recommended)

### Bootstrap Script Fails
```bash
# Check yq is installed
yq --version

# Install yq if needed
sudo apt install yq

# Run with verbose output
bash -x ./bootstrap.sh
```

## Bootstrap Script Details

The `bootstrap.sh` script handles the complex startup sequence required for multiple vLLM models on a single GPU:

**Phase 1: Sequential Model Loading**
- Starts each model one at a time
- Polls `/health` endpoint (max 7.5 min per model)
- Immediately puts model to sleep after health check passes
- This prevents OOM by ensuring only one model uses VRAM at a time

**Phase 2: Activate Default Model**
- Wakes up the default model (defined in `config.yaml`)
- Waits for health check to confirm it's ready

**Phase 3: Start Management Services**
- Starts Model Manager (will resync and detect model states)
- Starts Open WebUI

**Why Bootstrap is Necessary:**
- Models cannot both be active simultaneously (VRAM constraints)
- Docker Compose's `depends_on` doesn't handle sleep/wake sequence
- Models must be cached before switching works properly
- Proper state initialization prevents race conditions

## Known Limitations

1. **Direct vLLM Endpoint Access**
   - Sending requests directly to sleeping vLLM endpoints causes crashes
   - **Workaround:** Only use Model Manager `/switch` API, not direct vLLM calls
   - **Future:** Model Router component will proxy all requests to active model

2. **Single GPU Only**
   - Current implementation assumes all models share one GPU
   - Multi-GPU support requires architecture changes

3. **No Request Queuing**
   - Requests during model switch are lost
   - **Future:** Queue requests during switch operations

4. **Manual WebUI Model Selection**
   - WebUI shows all model endpoints, but only active one works
   - **Future:** Dynamic model list based on Model Manager state

5. **Sleep Mode Requires Dev Mode**
   - `VLLM_SERVER_DEV_MODE=1` is required for sleep endpoints
   - Not recommended for production deployments per vLLM docs

## Future Enhancements

- [ ] WebSocket support for real-time status updates
- [ ] Open WebUI custom plugin for model selection UI
- [ ] Automatic model preloading on startup
- [ ] Model usage statistics and logging
- [ ] Support for multiple GPUs per model
- [ ] Graceful shutdown with state persistence
- [ ] Prometheus metrics export
- [ ] Admin dashboard for monitoring

- [ ] Admin dashboard for monitoring

## Prerequisites (Docker Setup)

- NVIDIA Driver (recommended: 525 or later)
- Docker Engine (23.0 or later)
- NVIDIA Container Toolkit
- Docker Compose V2

### Installing Prerequisites

1. Install NVIDIA Container Toolkit:
```bash
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg \
  && curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
    sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
    sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list

sudo apt-get update && sudo apt-get install -y nvidia-container-toolkit
sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker
```

2. Verify NVIDIA Docker installation:
```bash
docker run --rm --gpus all nvidia/cuda:12.3.0-base-ubuntu24.04 nvidia-smi
```

## Resources

- [vLLM Documentation](https://vllm.readthedocs.io/)
- [vLLM Sleep Mode Guide](https://docs.vllm.ai/en/latest/features/sleep_mode.html)
- [Open WebUI Documentation](https://github.com/open-webui/open-webui)
- [NVIDIA Container Toolkit Guide](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html)
- [Gin Web Framework](https://gin-gonic.com/)

## License

MIT License - See LICENSE file for details.