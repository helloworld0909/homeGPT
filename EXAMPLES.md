# HomeGPT Examples

## Starting the Stack

### Using the default model (qwen3-vl-30b)
```bash
cd docker
./run.sh up -d
```

### Using a specific model via command line
```bash
cd docker
MODEL_NAME=qwen3-vl-32b-thinking ./run.sh up -d
```

### Using a specific model via models.env
```bash
cd docker
# Edit models.env and set MODEL_NAME=qwen3-vl-32b-thinking
nano models.env
./run.sh up -d
```

## Switching Between Models

To switch from one model to another:

```bash
cd docker
# Stop the current stack
./run.sh down

# Edit models.env to select a different model
nano models.env

# Start with the new model
./run.sh up -d
```

Or use the command line:

```bash
cd docker
./run.sh down
MODEL_NAME=qwen3-vl-32b-thinking ./run.sh up -d
```

## Common Commands

```bash
# View all logs
./run.sh logs

# View logs for a specific service
./run.sh logs vllm
./run.sh logs webui

# Follow logs in real-time
./run.sh logs -f

# Restart services
./run.sh restart

# Stop everything
./run.sh down

# Check configuration
./run.sh config
```

## Adding a New Model

1. Edit `docker/models.env` and add your model configuration:

```bash
# Model: My Custom Model (my-custom-model)
MY_CUSTOM_MODEL_MODEL=organization/model-name
MY_CUSTOM_MODEL_GPU_MEMORY_UTILIZATION=0.90
MY_CUSTOM_MODEL_MAX_MODEL_LEN=50000
MY_CUSTOM_MODEL_EXTRA_ARGS=--tensor-parallel-size 2
```

2. Use the new model:

```bash
MODEL_NAME=my-custom-model ./run.sh up -d
```

Or edit `models.env` to set `MODEL_NAME=my-custom-model` and run `./run.sh up -d`.
