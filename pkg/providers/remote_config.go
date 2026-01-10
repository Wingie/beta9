package providers

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/beam-cloud/beta9/pkg/network"
	"github.com/beam-cloud/beta9/pkg/storage"
	"github.com/beam-cloud/beta9/pkg/types"
)

const connectTimeout time.Duration = time.Second * 5

func GetRemoteConfig(baseConfig types.AppConfig, tailscale *network.Tailscale) (*types.AppConfig, error) {
	configBytes, err := json.Marshal(baseConfig)
	if err != nil {
		return nil, err
	}

	// Overwrite certain config fields with tailscale hostnames
	// TODO: figure out a more elegant to override these fields without hardcoding service names
	remoteConfig := types.AppConfig{}
	if err = json.Unmarshal(configBytes, &remoteConfig); err != nil {
		return nil, err
	}

	// Determine Redis hostname - use direct IP if configured, otherwise Tailscale resolution
	var redisHostname string
	useDirectHost := baseConfig.Tailscale.DirectRedisHost != ""

	if useDirectHost {
		// Direct mode: use configured IP (bypasses Tailscale service discovery)
		redisHostname = baseConfig.Tailscale.DirectRedisHost
	} else {
		// Standard mode: resolve via Tailscale
		redisHostname, err = tailscale.ResolveService("control-plane-redis", connectTimeout)
		if err != nil {
			return nil, err
		}
	}

	// Use configured external port or default to 6379
	redisExternalPort := baseConfig.Database.Redis.ExternalPort
	if redisExternalPort == 0 {
		redisExternalPort = 6379
	}
	remoteConfig.Database.Redis.Addrs[0] = fmt.Sprintf("%s:%d", redisHostname, redisExternalPort)
	remoteConfig.Database.Redis.InsecureSkipVerify = true

	if baseConfig.Storage.Mode == storage.StorageModeJuiceFS {
		var juiceFsRedisHostname string

		if useDirectHost {
			// Direct mode: use same host for JuiceFS Redis
			juiceFsRedisHostname = baseConfig.Tailscale.DirectRedisHost
		} else {
			// Standard mode: resolve via Tailscale
			juiceFsRedisHostname, err = tailscale.ResolveService("juicefs-redis", connectTimeout)
			if err != nil {
				return nil, err
			}
		}

		// Use configured external port or default to 6379
		juicefsExternalPort := baseConfig.Storage.JuiceFS.ExternalRedisPort
		if juicefsExternalPort == 0 {
			juicefsExternalPort = 6379
		}

		parsedUrl, err := url.Parse(remoteConfig.Storage.JuiceFS.RedisURI)
		if err != nil {
			return nil, err
		}
		juicefsRedisPassword, _ := parsedUrl.User.Password()

		// Use redis:// (non-TLS) when using direct host - Tailscale already encrypts traffic
		// Use rediss:// (TLS) when using Tailscale DNS resolution
		if useDirectHost {
			remoteConfig.Storage.JuiceFS.RedisURI = fmt.Sprintf("redis://:%s@%s:%d/0",
				juicefsRedisPassword, juiceFsRedisHostname, juicefsExternalPort)

			// Update S3 bucket endpoint for external workers
			s3ExternalPort := baseConfig.Storage.JuiceFS.ExternalS3Port
			if s3ExternalPort == 0 {
				s3ExternalPort = 31566 // Default LocalStack NodePort
			}

			// Parse existing bucket URL to get bucket path (e.g., /juicefs)
			bucketPath := "/juicefs" // Default bucket path
			if parsedBucket, err := url.Parse(remoteConfig.Storage.JuiceFS.AWSS3Bucket); err == nil {
				bucketPath = parsedBucket.Path
			}

			remoteConfig.Storage.JuiceFS.AWSS3Bucket = fmt.Sprintf(
				"http://%s:%d%s",
				baseConfig.Tailscale.DirectRedisHost, // Reuse same Tailscale IP
				s3ExternalPort,
				bucketPath,
			)
		} else {
			remoteConfig.Storage.JuiceFS.RedisURI = fmt.Sprintf("rediss://:%s@%s:%d/0",
				juicefsRedisPassword, juiceFsRedisHostname, juicefsExternalPort)
		}
	}

	// Update worker image registry for external workers
	// Priority: ExternalImageRegistry (HTTPS hostname) > ExternalRegistryPort (IP:port)
	if useDirectHost {
		if baseConfig.Worker.ExternalImageRegistry != "" {
			// Use HTTPS registry hostname (e.g., registry.agentosaurus.com)
			remoteConfig.Worker.ImageRegistry = baseConfig.Worker.ExternalImageRegistry
		} else if baseConfig.Worker.ExternalRegistryPort > 0 {
			// Fallback to IP:port for HTTP registry via Tailscale
			remoteConfig.Worker.ImageRegistry = fmt.Sprintf(
				"%s:%d",
				baseConfig.Tailscale.DirectRedisHost,
				baseConfig.Worker.ExternalRegistryPort,
			)
		}

		// Update runner base image registry for external workers
		// External workers need HTTPS registry since they can't access localhost:5000
		if baseConfig.ImageService.Runner.ExternalBaseImageRegistry != "" {
			remoteConfig.ImageService.Runner.BaseImageRegistry = baseConfig.ImageService.Runner.ExternalBaseImageRegistry
		}
	}

	return &remoteConfig, nil
}
