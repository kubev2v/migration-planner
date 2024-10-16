# Agent service
The Agent service is responsible for receiving and serving the collected vCenter data to the user. Once the user creates a source for their vCenter environment, the Agent service will provide a streaming service to download an OVA image. The OVA image can be booted on the vCenter enviroment to perform the collection of the vCenter data.

## Agent API
There are two APIs related to the Agent.

### Internal API
The API contains operations to create a source, download the OVA image, etc. By default it runs on tcp port 3443. The API is not exposed externally to users, as it is only used internally by the UI.

### Agent API
The Agent API is exposed to communicate with the Agent VM. Its only operation is to update the status of the source. By default it runs on tcp port 7443. This API must be externally exposed so that the agent VM can initiate communication with it.
