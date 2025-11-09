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

2. Start the stack:
```bash
docker compose up -d
```

3. Access the services:
- OpenWebUI: http://localhost:8080
- vLLM API: http://localhost:8000

## Components

### vLLM Service
- Base Image: vllm/vllm-openai:v0.11.0-x86_64
- Model: QuantTrio/Qwen3-VL-30B-A3B-Instruct-AWQ
- Port: 8000
- GPU Memory Utilization: 95%

### OpenWebUI
- Base Image: ghcr.io/open-webui/open-webui:main
- Network Mode: Host
- API Base URL: http://localhost:8000/v1

## Configuration

The stack uses the following configuration in `docker-compose.yml`:

```yaml
services:
  vllm:
    - GPU configuration
    - Model settings
    - Memory utilization
  webui:
    - Network settings
    - API configuration
```

## Common Operations

### View Logs
```bash
# All services
docker compose logs

# Specific service
docker compose logs vllm
docker compose logs webui
```

### Restart Services
```bash
# Restart all
docker compose restart

# Restart specific service
docker compose restart vllm
docker compose restart webui
```

### Stop Stack
```bash
docker compose down
```

### Update Images
```bash
docker compose pull
docker compose up -d
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