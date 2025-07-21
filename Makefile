# Makefile for Telegram Archive

PROJECT ?= telegram-archive
COMPOSE_FILES       ?= docker-compose.yml
ENV_FILE            ?= .env
COMPOSE_CMD  := $(shell command -v docker-compose 2> /dev/null || echo "docker compose")

# Load environment if exists
ifneq ("$(wildcard $(ENV_FILE))","")
	include $(ENV_FILE)
	export
endif

# Argument handling for specific targets
ifneq (,$(filter log shell,$(firstword $(MAKECMDGOALS))))
  SERVICE := $(wordlist 2,$(words $(MAKECMDGOALS)),$(MAKECMDGOALS))
  $(eval $(SERVICE):;@:)
endif

# ANSI color codes
RED    := \033[0;31m
GREEN  := \033[0;32m
YELLOW := \033[0;33m
BLUE   := \033[0;34m
NC     := \033[0m


##@ Main Targets
.PHONY: help

help:  ## Show this help

	@echo "${GREEN}Telegram Archive Commands${NC}"
	@echo ""
	@echo "${YELLOW}Setup:${NC}"
	@echo "  make env        Create .env file from example"
	@echo ""
	@echo "${YELLOW}Building:${NC}"
	@echo "  make build           Build Docker images"
	@echo "  make build-up        Build and start containers"
	@echo "  make build-no-cache  Rebuild images without cache"
	@echo "  make build-manual    Build without Docker"
	@echo ""
	@echo "${YELLOW}Runtime:${NC}"
	@echo "  make run               Run manually"
	@echo "  make up                Start containers"
	@echo "  make stop              Stop containers"
	@echo "  make down              Stop and remove containers"
	@echo "  make status            Show running containers"
	@echo "  make log [service]     View container logs"
	@echo "  make shell [service]   Open shell in container"
	@echo ""
	@echo "${YELLOW}Cleanup:${NC}"
	@echo "  make purge     Remove all containers, images and volumes"
	@echo ""
	@echo "Run 'make <command>' to execute. Examples:"
	@echo "  ${BLUE}make build-up${NC}     # Build and start"
	@echo "  ${BLUE}make log bot${NC}      # View bot logs"
	@echo ""


default: help  ## Default target

.PHONY: env build run stop clean logs export purge log shell

env:
	@[ -e ./${ENV_FILE} ] || cp -v ./.env.example ./${ENV_FILE}

##@ Docker Operations
.PHONY: up build build-run build-no-cache status stop down purge

up: #Start containers in background
	@printf "${BLUE}Starting services...${NC}\n"
	@$(COMPOSE_CMD) -f "$(COMPOSE_FILES)" up -d

build:  #Build Docker images
	@printf "${BLUE}Building images...${NC}\n"
	@$(COMPOSE_CMD) -f "$(COMPOSE_FILES)" build

build-up: build up  # Build and start containers

build-no-cache:  # Build images without cache
	@printf "${BLUE}Building images (no cache)...${NC}\n"
	@$(COMPOSE_CMD) -f "$(COMPOSE_FILES)" build --no-cache

status:  # Show container status
	@printf "${BLUE}Container status:${NC}\n"
	@$(COMPOSE_CMD) -f "$(COMPOSE_FILES)" ps $(SERVICE)

stop:  # Stop running containers
	@printf "${BLUE}Stopping services...${NC}\n"
	@$(COMPOSE_CMD) -f "$(COMPOSE_FILES)" stop

down: stop  # Stop and remove containers (preserves data)
	@$(COMPOSE_CMD) -f "$(COMPOSE_FILES)" down

purge:  # Destroy all containers and volumes
	@printf "${RED}Purging all project artifacts...${NC}\n"
	@$(COMPOSE_CMD) -f "$(COMPOSE_FILES)" down -v --rmi all --remove-orphans

log:  # View container logs (specify service: make log bot)
	@printf "${BLUE}Tailing logs for $(or $(SERVICE),all services)...${NC}\n"
	@$(COMPOSE_CMD) -f "$(COMPOSE_FILES)" logs -f $(SERVICE)

shell:  # Access container shell (specify service: make shell bot)
	@printf "${BLUE}Opening shell in $(or $(SERVICE),default) container...${NC}\n"
	@$(COMPOSE_CMD) -f "$(COMPOSE_FILES)" exec $(SERVICE) /bin/bash


.PHONY: build-manual run

build-manual: env
	python -m venv .venv
	source ./.venv/bin/activate
	pip install -r requirements.txt

run:
	python bot.py
