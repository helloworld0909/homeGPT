#!/bin/bash

# Save the MODEL_NAME if provided via command line
CMDLINE_MODEL_NAME="${MODEL_NAME}"

# Load the models configuration
if [ -f "models.env" ]; then
    source models.env
else
    echo "Error: models.env file not found"
    exit 1
fi

# Use command line MODEL_NAME if provided, otherwise use the one from models.env
if [ -n "$CMDLINE_MODEL_NAME" ]; then
    MODEL_NAME="$CMDLINE_MODEL_NAME"
fi

# Set default model if not specified
MODEL_NAME=${MODEL_NAME:-qwen3-vl-30b}

# Function to set model parameters
set_model_params() {
    local model_key
    model_key=$(echo "$MODEL_NAME" | tr '[:lower:]-' '[:upper:]_')
    
    # Get the model parameters
    local vllm_model
    local vllm_gpu_mem
    local vllm_max_len
    local vllm_extra
    
    vllm_model=$(eval echo "\$${model_key}_MODEL")
    vllm_gpu_mem=$(eval echo "\$${model_key}_GPU_MEMORY_UTILIZATION")
    vllm_max_len=$(eval echo "\$${model_key}_MAX_MODEL_LEN")
    vllm_extra=$(eval echo "\$${model_key}_EXTRA_ARGS")
    
    # Validate that model was found
    if [ -z "$vllm_model" ]; then
        echo "Error: Model '$MODEL_NAME' not found in configuration"
        echo "Available models: qwen3-vl-30b, qwen3-vl-32b-thinking"
        exit 1
    fi
    
    # Export the model parameters
    export VLLM_MODEL="$vllm_model"
    export VLLM_GPU_MEMORY_UTILIZATION="$vllm_gpu_mem"
    export VLLM_MAX_MODEL_LEN="$vllm_max_len"
    export VLLM_EXTRA_ARGS="$vllm_extra"
    
    echo "Using model: $VLLM_MODEL"
    echo "GPU Memory Utilization: $VLLM_GPU_MEMORY_UTILIZATION"
    echo "Max Model Length: $VLLM_MAX_MODEL_LEN"
    if [ -n "$VLLM_EXTRA_ARGS" ]; then
        echo "Extra Args: $VLLM_EXTRA_ARGS"
    fi
}

# Set the model parameters
set_model_params

# Export variables for docker-compose
export MODEL_NAME
export VLLM_MODEL
export VLLM_GPU_MEMORY_UTILIZATION
export VLLM_MAX_MODEL_LEN
export VLLM_EXTRA_ARGS

# Run docker-compose with the provided arguments
docker compose "$@"
