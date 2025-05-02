package ports

import "net/http"

type LoadBalancerService interface {
	HandleRequest(w http.ResponseWriter, r *http.Request)
}
