# Content of the contrib folder

## Build and starting a developement environment

`dev.yml` is a playbook that builds and starts a complete dev environment:
- agent
- db
- collector
- planner-api

You can deploy it with:
```
ansible-playbook dev.yml
```

To destroy the env:
```
ansible-playbook dev.yml --extra-vars='{"state":"absent"}'
```

It has the following variables:
```
agent_ui_image: quay.io/kubev2v/planner-agent-ui:latest
agent_image: localhost/planner-agent:latest
forklift_validation_image: quay.io/kubev2v/forklift-validation:release-v2.6.4
collector_image: localhost/planner-collector:latest
planner_api_image: localhost/planner-api:latest
db_image: quay.io/sclorg/postgresql-12-c8s:latest
db_port: 5432
state: "{{ state | default('present') }}"
```

