package platform

import (
	"buildandbreak/platform/internal/accounts"
	bnbruntime "buildandbreak/platform/internal/runtime"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
)

// WorkloadPayload defines the allowlisted template contract shared by manual
// development provisioning and billing fulfillment. Core never sees Docker clients.
func WorkloadPayload(id, template string, memory, cpu, port int) map[string]any {
	p := map[string]any{"name": "bnb-service-" + id, "image": "alpine:3.20", "memory_bytes": int64(memory) * 1048576, "cpu_threads": cpu, "command": []string{"sh", "-c", "while true; do sleep 60; done"}}
	if template == "minecraft-java" {
		p["image"] = "itzg/minecraft-server:java21"
		delete(p, "command")
		p["environment"] = []string{"EULA=TRUE", "TYPE=VANILLA", "VERSION=1.21.1", fmt.Sprintf("MEMORY=%dM", memory-512), "ENABLE_RCON=false", "CREATE_CONSOLE_IN_PIPE=true"}
		p["port"] = port
		p["persistent_data"] = true
	}
	if template == "minecraft-bedrock" {
		p["image"] = "itzg/minecraft-bedrock-server:latest"
		delete(p, "command")
		p["environment"] = []string{"EULA=TRUE", "VERSION=LATEST", "SERVER_NAME=Build & Break Bedrock", "ENABLE_SSH=false", "TRANSPORT=nethernet", "ALLOW_LIST=false", fmt.Sprintf("SERVER_UDP_PORTS=%d-%d:32000-32015", port+1, port+16)}
		p["port"] = port
		p["container_port"] = 19132
		p["port_protocol"] = "tcp"
		extra := []bnbruntime.PortBinding{}
		for i := 0; i < 16; i++ {
			extra = append(extra, bnbruntime.PortBinding{HostPort: port + 1 + i, ContainerPort: 32000 + i, Protocol: "udp"})
		}
		p["extra_ports"] = extra
		p["persistent_data"] = true
	}
	return p
}

// Fulfill creates an allocation, service and queue job in the caller's transaction.
// The sandbox ledger uses this to commit payment and entitlement atomically.
func Fulfill(ctx context.Context, tx pgx.Tx, node, name, template string, memory, disk, cpu, port int, minecraft ...MinecraftConfig) (serverID, serviceID, jobID string, err error) {
	var ready bool
	var totalMem, totalDisk int64
	var totalCPU int
	err = tx.QueryRow(ctx, `SELECT COALESCE(last_seen_at>=NOW()-INTERVAL '90 seconds' AND runtime_available,false),total_memory,total_disk,cpu_threads FROM nodes WHERE id=$1 FOR UPDATE`, node).Scan(&ready, &totalMem, &totalDisk, &totalCPU)
	if err != nil || !ready {
		return "", "", "", fmt.Errorf("node is unavailable")
	}
	var usedMem, usedDisk, usedCPU int64
	err = tx.QueryRow(ctx, `SELECT COALESCE(SUM(memory_mb),0),COALESCE(SUM(disk_mb),0),COALESCE(SUM(cpu_threads),0) FROM servers WHERE node_id=$1 AND status!='deleted'`, node).Scan(&usedMem, &usedDisk, &usedCPU)
	if err != nil {
		return
	}
	if (usedMem+int64(memory))*1048576 > totalMem || (usedDisk+int64(disk))*1048576 > totalDisk || usedCPU+int64(cpu) > int64(totalCPU) {
		err = fmt.Errorf("insufficient node capacity")
		return
	}
	if (template == "minecraft-java" || template == "minecraft-bedrock") && port == 0 {
		basePort := 25565
		if template == "minecraft-bedrock" {
			basePort = 19132
		}
		err = tx.QueryRow(ctx, `SELECT p FROM generate_series($2::integer,$2::integer+100) p WHERE NOT EXISTS(SELECT 1 FROM service_ports WHERE node_id=$1 AND port BETWEEN p AND p+CASE WHEN $3='minecraft-bedrock' THEN 16 ELSE 0 END) ORDER BY p LIMIT 1`, node, basePort, template).Scan(&port)
		if err != nil {
			err = fmt.Errorf("no game ports available on this node")
			return
		}
	}
	err = tx.QueryRow(ctx, `INSERT INTO servers(id,node_id,name,status,memory_mb,disk_mb,cpu_threads,account_id) VALUES(gen_random_uuid(),$1,$2,'active',$3,$4,$5,$6) RETURNING id`, node, name, memory, disk, cpu, accounts.Owner(ctx)).Scan(&serverID)
	if err != nil {
		return
	}
	err = tx.QueryRow(ctx, `INSERT INTO services(id,server_id,name,template,status,memory_mb,cpu_threads,port) VALUES(gen_random_uuid(),$1,$2,$3,'installing',$4,$5,$6) RETURNING id`, serverID, name, template, memory, cpu, nullablePort(port)).Scan(&serviceID)
	if err != nil {
		return
	}
	if port != 0 {
		_, err = tx.Exec(ctx, `INSERT INTO service_ports(node_id,port,service_id) VALUES($1,$2,$3)`, node, port, serviceID)
		if err != nil {
			err = fmt.Errorf("game port is already reserved")
			return
		}
	}
	payload := WorkloadPayload(serviceID, template, memory, cpu, port)
	if err = reserveExtraPorts(ctx, tx, node, serviceID, payload); err != nil {
		return
	}
	if len(minecraft) > 0 && template == "minecraft-java" {
		cfg := minecraft[0]
		if err = cfg.Validate(); err != nil {
			return
		}
		payload = MinecraftPayload(serviceID, memory, cpu, port, cfg)
		payload["minecraft_config"] = cfg
		b, _ := json.Marshal(cfg)
		if _, err = tx.Exec(ctx, `UPDATE services SET minecraft_config=$2 WHERE id=$1`, serviceID, b); err != nil {
			return
		}
	}
	jobID, err = enqueue(ctx, tx, node, serverID, serviceID, "create_container", payload)
	return
}

func reserveExtraPorts(ctx context.Context, tx pgx.Tx, node, service string, payload map[string]any) error {
	ports, ok := payload["extra_ports"].([]bnbruntime.PortBinding)
	if !ok {
		return nil
	}
	for _, p := range ports {
		if _, err := tx.Exec(ctx, `INSERT INTO service_ports(node_id,port,service_id) VALUES($1,$2,$3)`, node, p.HostPort, service); err != nil {
			return fmt.Errorf("extra game port is already reserved")
		}
	}
	data, _ := json.Marshal(ports)
	_, err := tx.Exec(ctx, `UPDATE services SET extra_ports=$2 WHERE id=$1`, service, data)
	return err
}
