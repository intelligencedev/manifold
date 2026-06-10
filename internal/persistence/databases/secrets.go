package databases

import (
	"fmt"
	"regexp"
	"strings"

	"manifold/internal/persistence"
	"manifold/internal/secrets"
)

var secretMapKeyPattern = regexp.MustCompile(`(?i)(^|[_\-.])(api[_\-.]?key|key|secret|token|password|credential|auth)([_\-.]|$)`)

func databaseSecretCodec(codec secrets.Codec) (secrets.Codec, error) {
	if codec != nil {
		return codec, nil
	}
	return secrets.NewCodecFromEnv()
}

func specialistSecretAAD(userID int64, name, field string) string {
	return fmt.Sprintf("specialists/%d/%s/%s", userID, name, field)
}

func mcpSecretAAD(userID int64, name, field string) string {
	return fmt.Sprintf("mcp_servers/%d/%s/%s", userID, name, field)
}

func secretSubfieldAAD(base, key string) string {
	return base + "/" + key
}

func sealSecretString(codec secrets.Codec, value, aad, label string) (string, error) {
	sealed, err := codec.SealString(value, aad)
	if err != nil {
		return "", fmt.Errorf("encrypt %s: %w", label, err)
	}
	return sealed, nil
}

func openSecretString(codec secrets.Codec, value, aad, label string) (string, error) {
	opened, err := codec.OpenString(value, aad)
	if err != nil {
		return "", fmt.Errorf("decrypt %s: %w", label, err)
	}
	return opened, nil
}

func sealAllStringMap(codec secrets.Codec, in map[string]string, aadBase, label string) (map[string]string, error) {
	if in == nil {
		return nil, nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		sealed, err := sealSecretString(codec, value, secretSubfieldAAD(aadBase, key), label+"."+key)
		if err != nil {
			return nil, err
		}
		out[key] = sealed
	}
	return out, nil
}

func openAllStringMap(codec secrets.Codec, in map[string]string, aadBase, label string) (map[string]string, error) {
	if in == nil {
		return nil, nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		opened, err := openSecretString(codec, value, secretSubfieldAAD(aadBase, key), label+"."+key)
		if err != nil {
			return nil, err
		}
		out[key] = opened
	}
	return out, nil
}

func sealSensitiveStringMap(codec secrets.Codec, in map[string]string, aadBase, label string) (map[string]string, error) {
	if in == nil {
		return nil, nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		if isSecretMapKey(key) {
			sealed, err := sealSecretString(codec, value, secretSubfieldAAD(aadBase, key), label+"."+key)
			if err != nil {
				return nil, err
			}
			out[key] = sealed
			continue
		}
		out[key] = value
	}
	return out, nil
}

func openSensitiveStringMap(codec secrets.Codec, in map[string]string, aadBase, label string) (map[string]string, error) {
	if in == nil {
		return nil, nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		if isSecretMapKey(key) {
			opened, err := openSecretString(codec, value, secretSubfieldAAD(aadBase, key), label+"."+key)
			if err != nil {
				return nil, err
			}
			out[key] = opened
			continue
		}
		out[key] = value
	}
	return out, nil
}

func isSecretMapKey(key string) bool {
	return secretMapKeyPattern.MatchString(strings.TrimSpace(key))
}

func encryptSpecialistForStore(codec secrets.Codec, userID int64, sp persistence.Specialist) (persistence.Specialist, error) {
	out := sp
	var err error
	out.APIKey, err = sealSecretString(codec, sp.APIKey, specialistSecretAAD(userID, sp.Name, "api_key"), "specialist api_key")
	if err != nil {
		return persistence.Specialist{}, err
	}
	out.ExtraHeaders, err = sealAllStringMap(codec, sp.ExtraHeaders, specialistSecretAAD(userID, sp.Name, "extra_headers"), "specialist extra_headers")
	if err != nil {
		return persistence.Specialist{}, err
	}
	return out, nil
}

func decryptSpecialistFromStore(codec secrets.Codec, sp persistence.Specialist) (persistence.Specialist, error) {
	out := sp
	var err error
	out.APIKey, err = openSecretString(codec, sp.APIKey, specialistSecretAAD(sp.UserID, sp.Name, "api_key"), "specialist api_key")
	if err != nil {
		return persistence.Specialist{}, err
	}
	out.ExtraHeaders, err = openAllStringMap(codec, sp.ExtraHeaders, specialistSecretAAD(sp.UserID, sp.Name, "extra_headers"), "specialist extra_headers")
	if err != nil {
		return persistence.Specialist{}, err
	}
	return out, nil
}

func encryptMCPServerForStore(codec secrets.Codec, userID int64, srv persistence.MCPServer) (persistence.MCPServer, error) {
	out := srv
	var err error
	out.BearerToken, err = sealSecretString(codec, srv.BearerToken, mcpSecretAAD(userID, srv.Name, "bearer_token"), "mcp bearer_token")
	if err != nil {
		return persistence.MCPServer{}, err
	}
	out.OAuthClientSecret, err = sealSecretString(codec, srv.OAuthClientSecret, mcpSecretAAD(userID, srv.Name, "oauth_client_secret"), "mcp oauth_client_secret")
	if err != nil {
		return persistence.MCPServer{}, err
	}
	out.OAuthAccessToken, err = sealSecretString(codec, srv.OAuthAccessToken, mcpSecretAAD(userID, srv.Name, "oauth_access_token"), "mcp oauth_access_token")
	if err != nil {
		return persistence.MCPServer{}, err
	}
	out.OAuthRefreshToken, err = sealSecretString(codec, srv.OAuthRefreshToken, mcpSecretAAD(userID, srv.Name, "oauth_refresh_token"), "mcp oauth_refresh_token")
	if err != nil {
		return persistence.MCPServer{}, err
	}
	out.Headers, err = sealAllStringMap(codec, srv.Headers, mcpSecretAAD(userID, srv.Name, "headers"), "mcp headers")
	if err != nil {
		return persistence.MCPServer{}, err
	}
	out.Env, err = sealSensitiveStringMap(codec, srv.Env, mcpSecretAAD(userID, srv.Name, "env"), "mcp env")
	if err != nil {
		return persistence.MCPServer{}, err
	}
	return out, nil
}

func decryptMCPServerFromStore(codec secrets.Codec, srv persistence.MCPServer) (persistence.MCPServer, error) {
	out := srv
	var err error
	out.BearerToken, err = openSecretString(codec, srv.BearerToken, mcpSecretAAD(srv.UserID, srv.Name, "bearer_token"), "mcp bearer_token")
	if err != nil {
		return persistence.MCPServer{}, err
	}
	out.OAuthClientSecret, err = openSecretString(codec, srv.OAuthClientSecret, mcpSecretAAD(srv.UserID, srv.Name, "oauth_client_secret"), "mcp oauth_client_secret")
	if err != nil {
		return persistence.MCPServer{}, err
	}
	out.OAuthAccessToken, err = openSecretString(codec, srv.OAuthAccessToken, mcpSecretAAD(srv.UserID, srv.Name, "oauth_access_token"), "mcp oauth_access_token")
	if err != nil {
		return persistence.MCPServer{}, err
	}
	out.OAuthRefreshToken, err = openSecretString(codec, srv.OAuthRefreshToken, mcpSecretAAD(srv.UserID, srv.Name, "oauth_refresh_token"), "mcp oauth_refresh_token")
	if err != nil {
		return persistence.MCPServer{}, err
	}
	out.Headers, err = openAllStringMap(codec, srv.Headers, mcpSecretAAD(srv.UserID, srv.Name, "headers"), "mcp headers")
	if err != nil {
		return persistence.MCPServer{}, err
	}
	out.Env, err = openSensitiveStringMap(codec, srv.Env, mcpSecretAAD(srv.UserID, srv.Name, "env"), "mcp env")
	if err != nil {
		return persistence.MCPServer{}, err
	}
	return out, nil
}
