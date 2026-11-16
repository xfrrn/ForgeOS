api_addr     = "https://127.0.0.1:8200"
cluster_addr = "https://127.0.0.1:8201"
disable_mlock = true

audit "file" "forgeos-file" {
  options {
    file_path = "/openbao/logs/forgeos-audit.jsonl"
    log_raw   = "false"
  }
}

storage "raft" {
  path    = "/openbao/file"
  node_id = "forgeos-local"
}

listener "tcp" {
  address         = "0.0.0.0:8200"
  cluster_address = "127.0.0.1:8201"
  tls_cert_file   = "/openbao/tls/server.crt"
  tls_key_file    = "/openbao/tls/server.key"
  tls_min_version = "tls12"
}
