package api

import (
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func NewRouter(h *Handlers, vpnH *VpnHandlers, routingH *RoutingHandlers, haH *HaHandlers) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", h.Health)

		r.Route("/providers", func(r chi.Router) {
			r.Get("/", h.ListProviders)
			r.Post("/", h.CreateProvider)
			r.Post("/import", h.ImportConfig)
			r.Get("/{id}", h.GetProvider)
			r.Put("/{id}", h.UpdateProvider)
			r.Delete("/{id}", h.DeleteProvider)
		})

		r.Route("/profiles", func(r chi.Router) {
			r.Get("/", h.ListProfiles)
			r.Post("/", h.CreateProfile)
			r.Get("/{id}", h.GetProfile)
			r.Put("/{id}", h.UpdateProfile)
			r.Delete("/{id}", h.DeleteProfile)
			r.Get("/{id}/rules", h.ListRules)
		})

		r.Route("/rules", func(r chi.Router) {
			r.Post("/", h.CreateRule)
			r.Get("/{id}", h.GetRule)
			r.Put("/{id}", h.UpdateRule)
			r.Delete("/{id}", h.DeleteRule)
		})

		r.Route("/interfaces", func(r chi.Router) {
			r.Get("/", h.ListInterfaces)
			r.Get("/{name}", h.GetInterface)
		})

		r.Get("/health-checks/{providerId}", h.ListHealthChecks)

		r.Route("/vpn", func(r chi.Router) {
			r.Get("/interfaces", vpnH.ListInterfaces)
			r.Post("/sync", vpnH.SyncProviders)
		})

		r.Route("/routing", func(r chi.Router) {
			r.Get("/status", routingH.RoutingStatus)
			r.Post("/deactivate", routingH.DeactivateRouting)
			r.Post("/reapply", routingH.ReapplyRouting)
		})

		r.Post("/profiles/{id}/activate", routingH.ActivateProfile)

		r.Route("/ha", func(r chi.Router) {
			r.Get("/status", haH.Status)
			r.Post("/activate/{id}", haH.Activate)
		})
	})

	return r
}
