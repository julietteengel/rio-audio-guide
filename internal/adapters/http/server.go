package http

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"rioaudioguide/backend/internal/ports"
)

// Server regroupe l'instance Echo et les ports dont l'API a besoin (les adaptateurs ne doivent jamais se connaître entre eux, seulement connaître les ports.)
type Server struct {
	echo          *echo.Echo
	placeRepo     ports.PlaceRepository
	scriptRepo    ports.ScriptRepository
	audioFileRepo ports.AudioFileRepository
	userRepo      ports.UserRepository
	publisher     ports.AudioJobPublisher
	storage       ports.AudioStorage
	cache         ports.Cache
	tokens        ports.TokenIssuer
}

func NewServer(placeRepo ports.PlaceRepository, scriptRepo ports.ScriptRepository, audioFileRepo ports.AudioFileRepository, userRepo ports.UserRepository, publisher ports.AudioJobPublisher, storage ports.AudioStorage, cache ports.Cache, tokens ports.TokenIssuer) *Server {
	s := &Server{
		echo:          echo.New(),
		placeRepo:     placeRepo,
		scriptRepo:    scriptRepo,
		audioFileRepo: audioFileRepo,
		userRepo:      userRepo,
		publisher:     publisher,
		storage:       storage,
		cache:         cache,
		tokens:        tokens,
	}
	// Sans ce middleware, un navigateur (web/, mobile/ en cible web via
	// react-native-web) bloque toute réponse de cette API -- curl et l'app
	// mobile native, eux, n'appliquent jamais CORS, donc rien ne l'aurait
	// révélé avant de tester depuis un vrai navigateur. AllowOrigins("*")
	// est sûr ici précisément parce que l'auth passe par un header
	// Authorization (JWT), jamais par un cookie -- pas de credentials
	// ambiants qu'une origine tierce pourrait siphonner. À resserrer sur
	// les domaines réels avant une vraie prod publique.
	s.echo.Use(middleware.CORS())

	s.echo.GET("/places", s.listPlaces)
	s.echo.GET("/places/:id", s.getPlaceDetail)
	s.echo.GET("/places/:id/audio", s.getPlaceAudio)
	s.echo.GET("/cities/:city/manifest", s.getCityManifest)

	auth := requireAuth(s.tokens)
	s.echo.POST("/scripts/:id/review", s.reviewScript, auth)

	s.echo.POST("/register", s.registerUser)
	s.echo.POST("/login", s.login)
	s.echo.POST("/logout", s.logout, auth)
	s.echo.PATCH("/me", s.updateMe, auth)
	s.echo.DELETE("/me", s.deleteMe, auth)
	return s
}

func (s *Server) Start(addr string) error {
	return s.echo.Start(addr)
}

const cacheTTL = 5 * time.Minute

// cachedJSON essaie le cache d'abord ; sur miss ou erreur (fail-open), appelle
// compute, sert le résultat, et tente de le mettre en cache pour la prochaine
// fois — une erreur de cache (lecture comme écriture) est loguée (pas juste
// ignorée) mais ne fait jamais échouer la requête. Sans ce log, un Redis mal
// configuré ou injoignable resterait invisible indéfiniment : le fail-open
// rend chaque réponse correcte, juste jamais servie depuis le cache.
func (s *Server) cachedJSON(c echo.Context, key string, compute func() (any, int, error)) error {
	cached, found, err := s.cache.Get(c.Request().Context(), key)
	if err != nil {
		log.Printf("cache get failed for key %q: %v", key, err)
	}
	if err == nil && found {
		return c.JSONBlob(http.StatusOK, []byte(cached))
	}

	value, status, err := compute()
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return c.JSON(status, value)
	}

	body, err := json.Marshal(value)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}
	if err := s.cache.Set(c.Request().Context(), key, string(body), cacheTTL); err != nil {
		log.Printf("cache set failed for key %q: %v", key, err) // fail-open : logué, jamais fatal
	}
	return c.JSONBlob(http.StatusOK, body)
}
