package napcat

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
)

type openAPISpec struct {
	Paths map[string]map[string]openAPIOperation `json:"paths"`
}
type openAPIOperation struct {
	Summary string   `json:"summary"`
	Tags    []string `json:"tags"`
}

func LoadCapabilities(path string) ([]domain.Capability, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read NapCat OpenAPI: %w", err)
	}
	var spec openAPISpec
	if err := json.Unmarshal(b, &spec); err != nil {
		return nil, fmt.Errorf("parse NapCat OpenAPI: %w", err)
	}
	result := make([]domain.Capability, 0, len(spec.Paths))
	for endpoint, methods := range spec.Paths {
		for method, operation := range methods {
			method = normalizeMethod(method)
			if method == "" {
				continue
			}
			tag := ""
			if len(operation.Tags) > 0 {
				tag = operation.Tags[0]
			}
			readOnly := method == "GET" || isReadOnlyEndpoint(endpoint)
			result = append(result, domain.Capability{Endpoint: endpoint, Method: method, Summary: operation.Summary, Tag: tag, ReadOnly: readOnly, RequiresConfirmation: !readOnly, RequiresAdmin: isAdminEndpoint(endpoint), Implemented: isImplementedEndpoint(endpoint)})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Endpoint < result[j].Endpoint })
	return result, nil
}

func isImplementedEndpoint(endpoint string) bool {
	for _, value := range []string{"/get_login_info", "/get_group_list", "/get_friend_list", "/get_group_member_list", "/get_group_msg_history", "/get_status", "/get_version_info"} {
		if endpoint == value {
			return true
		}
	}
	return false
}

func normalizeMethod(method string) string {
	switch method {
	case "get", "GET":
		return "GET"
	case "post", "POST":
		return "POST"
	case "put", "PUT":
		return "PUT"
	case "patch", "PATCH":
		return "PATCH"
	case "delete", "DELETE":
		return "DELETE"
	}
	return ""
}

func isReadOnlyEndpoint(endpoint string) bool {
	for _, prefix := range []string{"/get_", "/fetch_", "/download_", "/check_", "/can_", "/nc_get_", "/_get_", "/ocr_", "/.ocr_"} {
		if len(endpoint) >= len(prefix) && endpoint[:len(prefix)] == prefix {
			return true
		}
	}
	return endpoint == "/get_msg" || endpoint == "/get_status" || endpoint == "/get_version_info"
}
func isAdminEndpoint(endpoint string) bool {
	for _, p := range []string{"/set_group_", "/delete_", "/send_", "/upload_", "/move_", "/rename_", "/trans_", "/clean_", "/set_restart", "/bot_exit", "/send_packet", "/set_qq_", "/set_friend_"} {
		if len(endpoint) >= len(p) && endpoint[:len(p)] == p {
			return true
		}
	}
	return false
}
