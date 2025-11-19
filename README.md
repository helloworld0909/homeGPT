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
- vLLM Qwen3-VL-30B: http://localhost:8001
- vLLM Qwen3-VL-32B: http://localhost:8002
- vLLM Qwen3-Next-80B: http://localhost:8003
- vLLM GPT-OSS-20B: http://localhost:8004

## Test Model Switching

```bash
# Check current status
curl -s http://localhost:9000/models | jq

# Switch to GPT-OSS
curl -X POST http://localhost:9000/switch \
  -H "Content-Type: application/json" \
  -d '{"model_id": "gpt-oss-20b"}' | jq

# Switch to Qwen3-VL-32B
curl -X POST http://localhost:9000/switch \
  -H "Content-Type: application/json" \
  -d '{"model_id": "qwen3-vl-32b"}' | jq
```

## Architecture Overview

```
┌─────────────────┐
│   Open WebUI    │ (Port 3000)
└────────┬────────┘
         │
         ▼
┌─────────────────┐     ┌──────────────────────────┐
│ Model Manager   │────▶│ vLLM Qwen3-VL-30B        │ (Port 8001)
│   (Go Service)  │     │ [Active/Sleep]           │
│   Port 9000     │     └──────────────────────────┘
└─────────────────┘     ┌──────────────────────────┐
                        │ vLLM Qwen3-VL-32B        │ (Port 8002)
                        │ [Active/Sleep]           │
                        └──────────────────────────┘
                        ┌──────────────────────────┐
                        │ vLLM Qwen3-Next-80B      │ (Port 8003)
                        │ [Active/Sleep]           │
                        └──────────────────────────┘
                        ┌──────────────────────────┐
                        │ vLLM GPT-OSS-20B         │ (Port 8004)
                        │ [Active/Sleep]           │
                        └──────────────────────────┘
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
│   ├── compose-vllm-qwen3-vl-30b-a3b.yml
│   ├── compose-vllm-qwen3-vl-32b.yml
│   ├── compose-vllm-qwen3-next-80b-a3b-thinking.yml
│   ├── compose-vllm-gpt-oss-20b.yml
│   └── compose-webui.yml
├── logs/                   # vLLM server logs
│   ├── qwen3-vl-30b-a3b/
│   ├── qwen3-vl-32b/
│   ├── qwen3-next-80b-a3b-thinking/
│   └── gpt-oss-20b/
├── config.yaml             # Model Manager configuration
├── vllm-logging.json       # vLLM logging configuration
├── bootstrap.sh            # Automated startup script
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

Follow these steps to add a new vLLM model to the system. Example: Adding Qwen3-VL-32B-Instruct-FP8.

### Step 1: Create Docker Compose File

Create `docker/compose-vllm-<model-name>.yml` following the naming convention:

```yaml
services:
  vllm-<model-name>:
    image: vllm/vllm-openai:nightly-x86_64  # Use nightly for latest features
    container_name: vllm-<model-name>
    restart: unless-stopped
    command: >
      <HuggingFace/Model-Name>              # Model identifier from HuggingFace
      --gpu-memory-utilization 0.90         # Adjust based on model size
      --max-model-len 262144                # Context window (adjust as needed)
      --max-num-batched-tokens 49152        # Batch size (adjust as needed)
      --kv-cache-dtype fp8                  # Use fp8 for memory efficiency
      --tensor-parallel-size 2              # Number of GPUs (1 or 2)
      --enable-chunked-prefill              # Enable for long context
      --enable-sleep-mode                   # Required for hot-swapping
      --enable-auto-tool-choice             # Enable tool calling if supported
      --tool-call-parser hermes             # Tool parser (if applicable)
    ports:
      - "<host-port>:8000"                  # Choose next available host port
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 2                      # Match tensor-parallel-size
              capabilities: [gpu]
    volumes:
      - ~/.cache/huggingface:/root/.cache/huggingface
      - ../vllm-logging.json:/logs/logging.json:ro
      - ../logs/<model-name>:/logs
      - ~/.cache/vllm:/root/.cache/vllm
      - ~/.cache/triton:/root/.triton/cache
      - ~/.cache/flashinfer:/root/.cache/flashinfer
    environment:
      - HUGGING_FACE_HUB_TOKEN=${HF_TOKEN}
      - VLLM_SERVER_DEV_MODE=1              # Required for sleep mode
      - CUDA_VISIBLE_DEVICES=0,1            # GPU indices (adjust as needed)
      - VLLM_HOST_IP=127.0.0.1
      - VLLM_CONFIGURE_LOGGING=1
      - VLLM_LOGGING_CONFIG_PATH=/logs/logging.json
    ipc: host
    networks:
      - homegpt-network
```

**Example (Qwen3-VL-32B):**
```yaml
services:
  vllm-qwen3-vl-32b:
    image: vllm/vllm-openai:nightly-x86_64
    container_name: vllm-qwen3-vl-32b
    restart: unless-stopped
    command: >
      Qwen/Qwen3-VL-32B-Instruct-FP8
      --gpu-memory-utilization 0.90
      --max-model-len 262144
      --max-num-batched-tokens 49152
      --kv-cache-dtype fp8
      --tensor-parallel-size 2
      --enable-chunked-prefill
      --enable-sleep-mode 
      --enable-auto-tool-choice 
      --tool-call-parser hermes
    ports:
      - "8002:8000"
    # ... rest of config
```

### Step 2: Create Logs Directory

Create the logs directory with a basic logging.json:

```bash
mkdir -p logs/<model-name>
echo '{}' > logs/<model-name>/logging.json
```

### Step 3: Update Main Docker Compose

Edit `docker/docker-compose.yml` to include the new model service:

```yaml
name: home-gpt

include:
  - compose-model-manager.yml
  - compose-vllm-qwen3-vl-30b-a3b.yml
  - compose-vllm-qwen3-vl-32b.yml      # Add new model here
  - compose-vllm-qwen3-next-80b-a3b-thinking.yml
  - compose-vllm-gpt-oss-20b.yml
  - compose-webui.yml

networks:
  homegpt-network:
    driver: bridge
```

### Step 4: Update Model Manager Configuration

Edit `config.yaml` to add the model configuration:

```yaml
models:
  # ... existing models ...
  
  - id: <model-id>                      # Short identifier (e.g., qwen3-vl-32b)
    name: "<Display Name>"              # Human-readable name
    container_name: "vllm-<model-name>" # Must match Docker service name
    port: 8000                          # Internal container port (always 8000)
    host_port: <unique-port>            # External port (8001, 8002, 8003, etc.)
    gpu_memory_gb: <estimated-gb>       # Approximate GPU memory usage
    startup_mode: disabled              # Options: disabled | sleep | active
```

**Example:**
```yaml
  - id: qwen3-vl-32b
    name: "Qwen 3 VL 32B (FP8)"
    container_name: "vllm-qwen3-vl-32b"
    port: 8000
    host_port: 8002
    gpu_memory_gb: 60.0
    startup_mode: disabled
```

**Port Assignment Guidelines:**
- Group related models together (e.g., VL models first, then text models)
- Use sequential ports (8001, 8002, 8003, 8004, ...)
- Document the port mapping in comments if needed

### Step 5: Update WebUI Configuration

Edit `docker/compose-webui.yml` to expose the new model endpoint:

```yaml
environment:
  - OPENAI_API_BASE_URLS=http://vllm-model1:8000/v1;http://vllm-model2:8000/v1;http://vllm-newmodel:8000/v1
  - OPENAI_API_KEYS=dummy;dummy;dummy  # Add one 'dummy' per model
```

**Example:**
```yaml
environment:
  - OPENAI_API_BASE_URLS=http://vllm-qwen3-vl-30b-a3b:8000/v1;http://vllm-qwen3-vl-32b:8000/v1;http://vllm-gpt-oss-20b:8000/v1;http://vllm-qwen3-next-80b-a3b-thinking:8000/v1
  - OPENAI_API_KEYS=dummy;dummy;dummy;dummy
```

### Step 6: Start the New Model

```bash
cd docker

# Start the new model (it will download on first run)
docker compose up -d vllm-<model-name>

# Monitor logs to track download and initialization
docker compose logs -f vllm-<model-name>

# Restart WebUI to pick up the new endpoint
docker compose restart webui

# Optionally restart model-manager to reload config
docker compose restart model-manager
```

### Step 7: Test Model Switching

```bash
# Check model status
curl -s http://localhost:9000/models | jq

# Switch to the new model
curl -X POST http://localhost:9000/switch \
  -H "Content-Type: application/json" \
  -d '{"model_id": "<model-id>"}' | jq

# Verify it's active
curl -s http://localhost:9000/models | jq '.[] | select(.id=="<model-id>")'
```

### Common Model Configuration Parameters

**GPU Memory Utilization:**
- `0.90` - Default, safe for most models
- `0.95` - Aggressive, use for maximum context
- `0.85` - Conservative, if experiencing OOM

**Context Window (max-model-len):**
- `262144` - Ultra-long context (256K tokens)
- `131072` - Long context (128K tokens)
- `32768` - Standard (32K tokens)
- Reduce if experiencing OOM errors

**Tensor Parallelism:**
- `--tensor-parallel-size 1` - Single GPU
- `--tensor-parallel-size 2` - Two GPUs (common for 30B+ models)
- `--tensor-parallel-size 4` - Four GPUs (for 70B+ models)

**Quantization Support:**
- FP8/FP16 models work out of the box
- AWQ/GPTQ models are supported
- Adjust `--quantization` flag if needed

### Troubleshooting New Models

**Model download fails:**
```bash
# Check HuggingFace token
echo $HF_TOKEN

# Pre-download manually
huggingface-cli login
huggingface-cli download Org/Model-Name
```

**Out of memory during startup:**
- Reduce `--gpu-memory-utilization`
- Reduce `--max-model-len`
- Increase `--tensor-parallel-size` (use more GPUs)
- Use a quantized version (AWQ/GPTQ/FP8)

**Container won't start:**
```bash
# Check detailed logs
docker compose logs vllm-<model-name>

# Verify GPU access
docker run --rm --gpus all nvidia/cuda:12.3.0-base-ubuntu24.04 nvidia-smi

# Check port conflicts
netstat -tulpn | grep <host-port>
```

**Model switches but doesn't respond:**
- Ensure `VLLM_SERVER_DEV_MODE=1` is set
- Ensure `--enable-sleep-mode` is in the command
- Check health endpoint: `curl http://localhost:<host-port>/health`
- Verify model is awake: `curl http://localhost:<host-port>/is_sleeping`

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
  - id: "qwen3-vl-30b-a3b"              # Unique identifier
    name: "Qwen 3 VL 30B A3B (MoE AWQ)" # Display name
    container_name: "vllm-qwen3-vl-30b-a3b"  # Docker container name
    port: 8000                          # Internal container port
    host_port: 8001                     # External host-mapped port
    gpu_memory_gb: 57.0                 # GPU memory usage estimate
    startup_mode: active                # Initial status: disabled/sleep/active

  - id: "qwen3-vl-32b"
    name: "Qwen 3 VL 32B (FP8)"
    container_name: "vllm-qwen3-vl-32b"
    port: 8000
    host_port: 8002
    gpu_memory_gb: 60.0
    startup_mode: disabled

  # ... more models ...

switching:
  available_ram_gb: 128.0               # Total RAM for sleep level decision
  max_retries: 450                      # Health check retries (15 min max)
  health_check_interval_seconds: 2      # Seconds between retries
```

**Sleep Level Selection:**
- If `available_ram_gb >= 64`: Use Level 1 (offload to CPU RAM)
- Otherwise: Use Level 2 (discard weights)

**Startup Modes:**
- `disabled`: Container not started at all
- `sleep`: Container started, model loaded, immediately put to sleep
- `active`: Container started, model loaded, and ready to serve (only ONE model should be active)

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
huggingface-cli download openai/gpt-oss-20b

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