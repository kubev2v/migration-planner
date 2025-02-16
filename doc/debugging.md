# Discovery VM Troubleshooter

## Objective
This troubleshooter is designed to guide you through the steps necessary to resolve issues related to the Discovery VM deployment.
If you see the source status as "not-connected" in the migration assessment service or if the Discovery VM is not functioning 
as expected, follow these steps to diagnose and fix the problem.
## Troubleshooting steps

The following conditions must be met:

**1. Ensure that the virtual machine is powered on**  
Verify that the deployed VM is in a powered-on state.

**2. Verify Network Configuration**  
* If using DHCP networking: Ensure that your DHCP server is enabled.
* If using static IP, bridges, or bonded networking - Ensure that your configurations are correct.

**3. SSH into the Discovery VM Machine**  
Retrieve the IP address of the deployed Discovery VM. After turning on the Discovery VM, the IP address should be 
displayed in the CLI if the network interface is available.

Once you have the IP address, verify that you can access the machine using SSH. Run the following command in the CLI:  
`ssh -i <identity_file> core@<machine-ip>`

* The -i parameter specifies the private key that corresponds to the public key provided during the OVA generation.  
* Replace <identity_file> with the actual path to your private key.

**4. Unable to SSH into the Discovery VM through the network?**
* Verify that the previously used IP address is correct and that Steps 1 and 2 were completed successfully.
* Follow common troubleshooting steps such as:
  1. Ping the Discovery VM from your machine.
  2. Check firewall rules to ensure traffic to the Discovery VM is allowed.
After resolving any connectivity issues, proceed with Step 3 again.

## Now, depending on your case, proceed with the additional steps

### Case: Unable to Access the Login Page

Verify That the Discovery VM Components Are Running

Run the following command to check all containers:  
`podman ps -a`  
Expected output should be similar to:

```
| CONTAINER ID | IMAGE                                               | COMMAND               | CREATED       | STATUS        | PORTS | NAMES          |
| cc0a71a37c1b | quay.io/kubev2v/forklift-validation:release-v2.6.4  | run --server /usr...  | 2 minutes ago | Up 31 minutes |       | opa            |
| 70ad0a7cbdc5 | quay.io/kubev2v/migration-planner-agent:latest      | -config /agent/co...  | 2 minutes ago | Up 31 minutes |       | planner-agent  |
```

**Note:** If only the OPA is not running, you should still be able to use the agent and view the final report. However,
this report will be partial and may not include warnings regarding the migration to OpenShift.

**Case: The planner-agent container is running:**   
Use the Discovery VM UI
Navigate to: **`https://<machine-ip>:3333/login`**. You should see a login form asking for VMware credentials.
If the site isn't reachable, Ensure that traffic to port 3333 is allowed in the firewall for the destination.

**Case: The planner-agent container is not running:**  
Follow these steps to troubleshoot and resolve the issue:
1. Inspect the logs of the planner-agent container to identify any errors or issues that might have caused it to stop.  
```shell
journaltct --user -u planner-agent
systemctl --user status planner-agent
```  
Review the output for any error messages or indications of what went wrong.

2. Restart the planner-agent Container:
```shell
systemctl --user restart planner-agent
```
After restarting, check its status again with the following command:  
`podman ps -a`  
If the issue persist try restarting the Discovery VM.

If the previous troubleshooting steps did not resolve the issue with the planner-agent container, it's advisable to 
contact Red Hat Support for further assistance. Please submit a ticket, describe the issue and add the logs if possible. 

### Case: 'not-connected' state of source keeps appearing

**1. Verify the Connection to the Service** 

The planner-agent exposes a `/status` endpoint, providing information to the Discovery VM UI and components regarding:

* The status of the process and status information
* The connection status to the migration service

If you can access the login page but the source status still appears as 'not connected' in the service, 
Check the Discovery VM connection status:

Navigate to: **`https://<machine-ip>:3333/api/v1/status`**.  
You will likely see output similar to:

```json
{
"status": "waiting-for-credentials",
"connected": "false",
"statusInfo": "No credentials provided"
}
```

If the output indicates that the service is unreachable ("connected": "false"), follow these steps:
1. Recheck the network assigned to the VM's interface to ensure it is connected to a proper network with access to the
   public internet or the service network destination.
2. Inspect the network traffic from the Discovery VM to the migration assessment service hosted by Red Hat at console.redhat.com
3. Ensure that the necessary outbound connections are allowed through the firewall.
