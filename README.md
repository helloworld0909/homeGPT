# HomeGPT Docker Stack

A Docker Compose setup for running vLLM with Qwen3-VL model and OpenWebUI interface.

## Prerequisites

- NVIDIA Driver (recommended: 525 or later)
- Docker Engine (23.0 or later)
- NVIDIA Container Toolkit
- Docker Compose V2

### System Requirements

- OS: Linux (Ubuntu 24.04 LTS recommended)
- RAM: 32GB minimum
- Storage: 100GB+ free space
- GPU: NVIDIA GPU with enough VRAM (e.g., RTX 5090)

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

# Verify NVIDIA Docker installation:
```bash
docker run --rm --gpus all nvidia/cuda:12.3.0-base-ubuntu24.04 nvidia-smi
```

## Quick Start

1. Clone the repository:
```bash
git clone https://github.com/helloworld0909/homeGPT.git
cd homeGPT/docker
```

2. (Optional) Select a model by editing `models.env`:
```bash
# Edit models.env and set MODEL_NAME to your desired model
# Available models: qwen3-vl-30b, qwen3-vl-32b-thinking
nano models.env
```

3. Start the stack:
```bash
./run.sh up -d
```

4. Access the services:
- OpenWebUI: http://localhost:8080
- vLLM API: http://localhost:8000

## Components

### vLLM Service
- Base Image: vllm/vllm-openai:v0.11.0-x86_64
- Default Model: QuantTrio/Qwen3-VL-30B-A3B-Instruct-AWQ
- Port: 8000
- Default GPU Memory Utilization: 95%

### Supported Models
The stack supports multiple models through the `models.env` configuration:

1. **qwen3-vl-30b** (default)
   - Model: QuantTrio/Qwen3-VL-30B-A3B-Instruct-AWQ
   - GPU Memory: 95%
   - Max Model Length: 100000

2. **qwen3-vl-32b-thinking**
   - Model: cpatonn/Qwen3-VL-32B-Thinking-AWQ-4bit
   - GPU Memory: 95%
   - Max Model Length: 100000

To add more models, edit `docker/models.env` and add the model configuration following the existing pattern.

### OpenWebUI
- Base Image: ghcr.io/open-webui/open-webui:main
- Network Mode: Host
- API Base URL: http://localhost:8000/v1

## Configuration

### Selecting a Model

The stack uses a `models.env` file to configure which model to use. To change models:

1. Edit `docker/models.env`
2. Set `MODEL_NAME` to one of the available models:
   - `qwen3-vl-30b` (default)
   - `qwen3-vl-32b-thinking`
3. Restart the stack: `./run.sh restart`

### Adding New Models

To add a new model, edit `docker/models.env` and add a new configuration block:

```bash
# Model: your-model-name (model-key)
MODEL_KEY_MODEL=huggingface/model-name
MODEL_KEY_GPU_MEMORY_UTILIZATION=0.95
MODEL_KEY_MAX_MODEL_LEN=100000
MODEL_KEY_EXTRA_ARGS=
```

Then set `MODEL_NAME=model-key` to use the new model.

### Customizing vLLM Parameters

Each model can have additional vLLM parameters using the `EXTRA_ARGS` field. For example:

```bash
# Add tensor parallel size and enforce eager execution
QWEN3_VL_32B_THINKING_EXTRA_ARGS=--tensor-parallel-size 2 --enforce-eager

# Add custom chat template
MODEL_KEY_EXTRA_ARGS=--chat-template /path/to/template.jinja
```

For a full list of available vLLM parameters, see the [vLLM documentation](https://vllm.readthedocs.io/).

### Docker Compose Configuration

The stack uses the following configuration in `docker-compose.yml`:

```yaml
services:
  vllm:
    - GPU configuration
    - Model settings (from environment variables)
    - Memory utilization (from environment variables)
  webui:
    - Network settings
    - API configuration
```

## Common Operations

### Switching Models

```bash
# Edit models.env and change MODEL_NAME
nano docker/models.env

# Restart the stack
cd docker
./run.sh restart
```

### View Logs
```bash
# All services
./run.sh logs

# Specific service
./run.sh logs vllm
./run.sh logs webui
```

### Restart Services
```bash
# Restart all
./run.sh restart

# Restart specific service
./run.sh restart vllm
./run.sh restart webui
```

### Stop Stack
```bash
./run.sh down
```

### Update Images
```bash
./run.sh pull
./run.sh up -d
```

## Troubleshooting

### Common Issues

1. GPU Not Detected
```bash
# Check NVIDIA driver
nvidia-smi

# Verify NVIDIA Docker
docker run --rm --gpus all nvidia/cuda:12.3.0-base-ubuntu24.04 nvidia-smi
```

2. Memory Issues
- Adjust `--gpu-memory-utilization` in docker-compose.yml (default: 0.95, 95% of GPU memory)
- Modify `--max-model-len` to control sequence length (default: 100000)
  - Lower values reduce memory usage but limit context window
  - Higher values allow longer conversations but require more GPU memory

3. Connection Issues
- Check if ports are available:
```bash
sudo lsof -i :8000
sudo lsof -i :8080
```

### Viewing Logs
```bash
# Live logs
docker compose logs -f

# Last 100 lines
docker compose logs --tail=100
```

## Resources

- [vLLM Documentation](https://vllm.readthedocs.io/)
- [OpenWebUI Documentation](https://github.com/open-webui/open-webui)
- [NVIDIA Container Toolkit Guide](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html)

## License

This project is open source and available under the MIT License.