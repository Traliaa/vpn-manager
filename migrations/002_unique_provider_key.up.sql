CREATE UNIQUE INDEX idx_vpn_providers_unique_key
ON vpn_providers(provider_type, (config->'peer'->>'public_key'))
WHERE provider_type IN ('amneziawg', 'wireguard')
  AND config->'peer'->>'public_key' IS NOT NULL
  AND config->'peer'->>'public_key' <> '';
