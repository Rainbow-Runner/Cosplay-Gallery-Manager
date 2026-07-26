package productapi

import (
	"context"
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/stashapp/stash/internal/persistence/productdb"
)

// AuthorizeRequest applies the product's single-owner authentication before
// GraphQL execution. Browse visibility is still enforced again by each store.
type AuthorizeRequest func(*http.Request) bool

type OperationsService interface {
	CreateFullBackup(context.Context) (productdb.BackupRecord, error)
	RestoreBackup(context.Context, string) (productdb.MaintenanceState, error)
}

func NewHandler(database *productdb.Database, authorize AuthorizeRequest) http.Handler {
	return NewHandlerWithOperations(database, authorize, nil)
}

func NewHandlerWithOperations(database *productdb.Database, authorize AuthorizeRequest, operations OperationsService) http.Handler {
	server := handler.New(NewExecutableSchema(Config{Resolvers: &Resolver{Database: database, Operations: operations}}))
	server.AddTransport(transport.Options{})
	server.AddTransport(transport.POST{})
	server.Use(extension.Introspection{})

	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if authorize == nil || !authorize(request) {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		server.ServeHTTP(w, request)
	})
}
