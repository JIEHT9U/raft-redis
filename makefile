DOCKER_IMAGE     := jieht9u/go-redis-raft
DOCKERFILE_LOCAL := Dockerfile

GITVERSION		 := 0.0.1


.PHONY: docker
docker: docker-build docker-push
	@echo "Success Docker"

.PHONY: docker-build
docker-build:
	@echo "Docker Build..."
	$Q docker build -t $(DOCKER_IMAGE):$(GITVERSION) --file=$(DOCKERFILE_LOCAL) .

.PHONY: docker-push
docker-push:
	@echo "Docker Push..."
	$Q docker tag $(DOCKER_IMAGE):$(GITVERSION) $(DOCKER_IMAGE):$(GITVERSION) 
	$Q docker push $(DOCKER_IMAGE):$(GITVERSION)