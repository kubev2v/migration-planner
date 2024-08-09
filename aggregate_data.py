import json
import argparse

def get_value_in_GB(val):
    return  round(val / (1024 ** 3), 2)

def infra_host_per_cluster(esxis_information):
    host_per_clsusters = []
    for cluster in esxis_information:
        print(len(cluster["hosts_info"]))
        host_per_clsusters.append(len(cluster["hosts_info"]))
    return host_per_clsusters

def s_pg(standard_pg_info):
    vlans_per_clusters = []
    for pg_per_cluster in standard_pg_info:
        vlans = []
        hosts_portgroup_info = pg_per_cluster["hosts_portgroup_info"]
        for host in hosts_portgroup_info:
            pgs = hosts_portgroup_info[host]
            for pg in pgs:
                if pg["vlan_id"] not in vlans:
                    vlans.append(pg["vlan_id"])

        vlans_per_clusters.append(vlans)

    return vlans_per_clusters

def infra_datastore(datastore_information):
    datastores = []
    for ds in datastore_information:
        datastores.append({
            "type": ds["type"],
            "totalCapacityGB": get_value_in_GB(ds["capacity"]),
            "freeCapacityGB": get_value_in_GB(ds["free_space"])
        })
    return datastores

def infra_networks(networks_information):
    networks = []
    for network in networks_information:
        networks.append({
            "type": network["type"],
            "name": network["name"]
        })
    return networks

def infra_host_powerstate(esxi_hosts):
    power_state = {}
    for host in esxi_hosts:
        if host['power_state'] not in power_state:
            power_state[host['power_state']] = 0
        power_state[host['power_state']] = power_state[host['power_state']] + 1
    return power_state


def vms(vm_details, validator):
    migrateable_vms_data = migrateable_vms(validator)

    total_vms = len(vm_details)
    total_memory = {
        "total": 0,
        "total_for_migrateable": 0,
        "total_for_migrateable_with_warnings": 0,
        "total_for_not_migrateable": 0
    }
    total_cpu = {
        "total": 0,
        "total_for_migrateable": 0,
        "total_for_migrateable_with_warnings": 0,
        "total_for_not_migrateable": 0
    }
    total_disk_GB = {
        "total": 0,
        "total_for_migrateable": 0,
        "total_for_migrateable_with_warnings": 0,
        "total_for_not_migrateable": 0
    }
    total_disk_count = {
        "total": 0,
        "total_for_migrateable": 0,
        "total_for_migrateable_with_warnings": 0,
        "total_for_not_migrateable": 0
    }

    power_state = {}
    guest_os = {}

    for vm in vm_details:
        vm_memory = vm['memory']['size_MiB']
        vm_cpu = vm['cpu']['count']
        total_disk_capacity = 0
        for vm_disk in vm['disks']:
            total_disk_capacity = total_disk_capacity + vm['disks'][vm_disk]['capacity']
        total_disk_capacity_GB = get_value_in_GB(total_disk_capacity)
        vm_disk_count = len(vm['disks'])
        # migrateable
        if vm["name"] in migrateable_vms_data["migratable_vms"]:
            total_memory["total_for_migrateable"] = total_memory["total_for_migrateable"] + vm_memory
            total_cpu["total_for_migrateable"] = total_cpu["total_for_migrateable"] + vm_cpu
            total_disk_GB["total_for_migrateable"] = total_disk_GB["total_for_migrateable"] + total_disk_capacity_GB
            total_disk_count["total_for_migrateable"] = total_disk_count["total_for_migrateable"] + vm_disk_count

        # migrateable with warnings
        if vm["name"] in migrateable_vms_data["migratable_vms_with_warnings"]:
            total_memory["total_for_migrateable_with_warnings"] = total_memory["total_for_migrateable_with_warnings"] + vm_memory
            total_cpu["total_for_migrateable_with_warnings"] = total_cpu["total_for_migrateable_with_warnings"] + vm_cpu
            total_disk_GB["total_for_migrateable_with_warnings"] = total_disk_GB["total_for_migrateable_with_warnings"] + total_disk_capacity_GB
            total_disk_count["total_for_migrateable_with_warnings"] = total_disk_count["total_for_migrateable_with_warnings"] + vm_disk_count

        # not migrateable
        if vm["name"] in migrateable_vms_data["not_migratable_vms"]:
            total_memory["total_for_not_migrateable"] = total_memory["total_for_not_migrateable"] + vm_memory
            total_cpu["total_for_not_migrateable"] = total_cpu["total_for_not_migrateable"] + vm_cpu
            total_disk_GB["not_migratable_vms"] = total_disk_GB["not_migratable_vms"] + total_disk_capacity_GB
            total_disk_count["not_migratable_vms"] = total_disk_count["not_migratable_vms"] + vm_disk_count

        total_memory["total"] = total_memory["total"] + vm_memory
        total_cpu["total"] = total_cpu["total"] + vm_cpu
        total_disk_GB["total"] = total_disk_GB["total"] + total_disk_capacity_GB
        total_disk_count["total"] = total_disk_count["total"] + vm_disk_count

        if vm['power_state'] not in power_state:
            power_state[vm['power_state']] = 0
        power_state[vm['power_state']] = power_state[vm['power_state']] +1
        if vm['guest_OS'] not in guest_os:
            guest_os[vm['guest_OS']] = 0
        guest_os[vm['guest_OS']] = guest_os[vm['guest_OS']] + 1

    cpu = {
        "total": total_cpu["total"],
        "totalForMigratable": total_cpu["total_for_migrateable"],
        "totalForMigratableWithWarnings": total_cpu["total_for_migrateable_with_warnings"],
        "totalForNotMigratable": total_cpu["total_for_not_migrateable"]
    }
    ram = {
        "total": total_memory["total"],
        "totalForMigratable": total_memory["total_for_migrateable"],
        "totalForMigratableWithWarnings": total_memory["total_for_migrateable_with_warnings"],
        "totalForNotMigratable": total_memory["total_for_not_migrateable"]
    }
    diskGB = {
        "total": total_disk_GB["total"],
        "totalForMigratable": total_disk_GB["total_for_migrateable"],
        "totalForMigratableWithWarnings": total_disk_GB["total_for_migrateable_with_warnings"],
        "totalForNotMigratable": total_disk_GB["total_for_not_migrateable"],

    }
    diskCount = {
        "total": total_disk_count["total"],
        "totalForMigratable": total_disk_count["total_for_migrateable"],
        "totalForMigratableWithWarnings": total_disk_count["total_for_migrateable_with_warnings"],
        "totalForNotMigratable": total_disk_count["total_for_not_migrateable"],

    }
    return {
        "total": total_vms,
        "totalMigratable": len(migrateable_vms_data["migratable_vms"]),
        "totalMigratableWithWarnings": len(migrateable_vms_data["migratable_vms_with_warnings"]),
        "totalNotMigratable": len(migrateable_vms_data["not_migratable_vms"]),
        "cpuCores": cpu,
        "ramGB": ram,
        "diskGB": diskGB,
        "diskCount": diskCount,
        "os": guest_os,
        "powerStates": power_state,
        "migrationWarnings": migrateable_vms_data["warnings"],
        "notMigratableReasons": migrateable_vms_data["errors"]
    }

def add_new_assessment_to_dict_if_needed(result, assessment_dict):
    if result["label"] not in assessment_dict:
        assessment_dict[result["label"]] = {
            "assessment": result["assessment"],
            "total_vms": 0
        }

    return assessment_dict

def migrateable_vms(validator):
    migratable_vms = {}
    migratable_vms_with_warnings = {}
    not_migratable_vms = {}
    warnings = {}
    errors = {}
    for vm_name in validator:
        migratable = True
        has_warning = False
        vm = validator[vm_name]["result"]
        for result in vm:
            # category can be one of: “Critical”, “Warning”, or “Information”
            if result["category"] == "Warning":
                has_warning = True
                warnings = add_new_assessment_to_dict_if_needed(result, warnings)
                warnings[result["label"]]["total_vms"] = warnings[result["label"]]["total_vms"] + 1

            if result["category"] == "Critical":
                migratable = False
                errors = add_new_assessment_to_dict_if_needed(result, errors)
                errors[result["label"]]["total_vms"] = errors[result["label"]]["total_vms"] + 1

        if migratable:
            migratable_vms[vm_name] = vm
        else:
            not_migratable_vms[vm_name] = vm
        if has_warning:
            migratable_vms_with_warnings[vm_name] = vm

    return {
        "migratable_vms": migratable_vms,
        "migratable_vms_with_warnings": migratable_vms_with_warnings,
        "not_migratable_vms": not_migratable_vms,
        "warnings": warnings,
        "errors": errors
    }

def aggregate_inventory_data(inventory, validator):
    vm_details = inventory["vm_details"]
    esxi_hosts = inventory["esxi_hosts"]
    datastore_information = inventory["datastore_information"]
    standard_pg_info = inventory["standard_pg_info"]
    clusters_information = inventory["clusters_information"]
    networks_information = inventory["networks_information"]
    esxis_information = inventory["esxis_information"]

    return {
        "inventory": {
            "vms": vms(vm_details, validator),
            "infra": {
                "datastores": infra_datastore(datastore_information),
                "totalHosts": len(esxi_hosts),
                "hostPowerStates": infra_host_powerstate(esxi_hosts),
                "totalClusters": len(clusters_information),
                "networks": infra_networks(networks_information),
                "totalHosts": len(esxi_hosts),
                "hostsPerCluster": infra_host_per_cluster(esxis_information),
                "standardVlanIDSPerCluster": s_pg(standard_pg_info),
                "distributedVlanIDS": "TBD",
            }
        }
    }

def main(inventory_file, validator_file, output_file):
    with open(inventory_file, 'r') as f:
        inventory = json.load(f)
    with open(validator_file, 'r') as f:
        validator = json.load(f)

    aggregated_data = aggregate_inventory_data(inventory, validator)

    print(aggregated_data)
    with open(output_file, 'w') as f:
        json.dump(aggregated_data, f, indent=4)

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description='Aggregate VM data.')
    parser.add_argument('inventory_file', type=str, help='Path to the output JSON file of the gather information.')
    parser.add_argument('validator_file', type=str, help='Path to the output JSON file of the validator.')
    parser.add_argument('output_file', type=str, help='Path to the output aggregation report JSON file.')
    args = parser.parse_args()
    main(args.inventory_file, args.validator_file, args.output_file)
