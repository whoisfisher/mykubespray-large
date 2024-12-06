#! /bin/bash
. .env
. check-env.sh
. common.sh


function configure_containerd() {
  current_ip=$(get_main_ip)
  echo "Configuring containerd client to allow insecure registry..."
  daemon_config="/etc/containerd/config.toml"
  if [ ! -f "$daemon_config" ]; then
    tee /etc/containerd/config.toml > /dev/null <<EOF
version = 2
root = "/var/lib/containerd"
state = "/run/containerd"
oom_score = 0



[grpc]
  max_recv_message_size = 16777216
  max_send_message_size = 16777216

[debug]
  level = "info"

[metrics]
  address = ""
  grpc_histogram = false

[plugins]
  [plugins."io.containerd.grpc.v1.cri"]
    sandbox_image = "$current_ip:$REGISTRY_PORT/pause:3.9"
    max_container_log_line_size = -1
    enable_unprivileged_ports = false
    enable_unprivileged_icmp = false
    [plugins."io.containerd.grpc.v1.cri".containerd]
      default_runtime_name = "runc"
      snapshotter = "overlayfs"
      discard_unpacked_layers = true
      [plugins."io.containerd.grpc.v1.cri".containerd.runtimes]
        [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
          runtime_type = "io.containerd.runc.v2"
          runtime_engine = ""
          runtime_root = ""
          base_runtime_spec = "/etc/containerd/cri-base.json"

          [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
            systemdCgroup = true
            binaryName = "/usr/local/bin/runc"
    [plugins."io.containerd.grpc.v1.cri".registry]
      [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
        [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
          endpoint = ["https://registry-1.docker.io"]
        [plugins."io.containerd.grpc.v1.cri".registry.mirrors."$current_ip:$REGISTRY_PORT"]
          endpoint = ["http://$current_ip:$REGISTRY_PORT"]
EOF
  fi
  cp /etc/containerd/config.toml .
}