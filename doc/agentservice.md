# Agent service
Agent service is responsible for serving collected data to the user. Once user create a source for his vCenter environment the Agent service provide a streaming service to download OVA image that is ready to be booted on the vCenter enviroment to run the collection of the data.

## Agent API
There are two APIs related to the Agent.

### Internal API
Internal Agent API exposed for the UI. This API contains operations to create source, download OVA, etc. By default running on port 3443. This API is not exposed externaly to users, it's used only internally by UI.

### Agent API
The Agent API is exposed for the communication with the Agent VM. The only operation is to update the status of the source. By default running on port 7443. This API must be externally exposed, so agent VM can send over data.
