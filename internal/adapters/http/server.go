package http

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"rioaudioguide/backend/internal/ports"
)

// Server regroupe l'instance Echo et les six ports dont l'API a besoin (les adaptateurs ne doivent jamais se connaître entre eux, seulement connaître les ports.)
type Server struct {
	echo          *echo.Echo
	placeRepo     ports.PlaceRepository
	scriptRepo    ports.ScriptRepository
	audioFileRepo ports.AudioFileRepository
	publisher     ports.AudioJobPublisher
	storage       ports.AudioStorage
	cache         ports.Cache
}

func NewServer(placeRepo ports.PlaceRepository, scriptRepo ports.ScriptRepository, audioFileRepo ports.AudioFileRepository, publisher ports.AudioJobPublisher, storage ports.AudioStorage, cache ports.Cache) *Server {
	s := &Server{
		echo:          echo.New(),
		placeRepo:     placeRepo,
		scriptRepo:    scriptRepo,
		audioFileRepo: audioFileRepo,
		publisher:     publisher,
		storage:       storage,
		cache:         cache,
	}
	s.echo.GET("/places", s.listPlaces)
	s.echo.GET("/places/:id", s.getPlaceDetail)
	s.echo.GET("/places/:id/audio", s.getPlaceAudio)
	s.echo.GET("/cities/:city/manifest", s.getCityManifest)
	s.echo.POST("/scripts/:id/review", s.reviewScript)
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
