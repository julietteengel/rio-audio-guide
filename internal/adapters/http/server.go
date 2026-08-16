package http

import (
	"github.com/labstack/echo/v4"

	"rioaudioguide/backend/internal/ports"
)

// Server regroupe l'instance Echo et les cinq ports dont l'API a besoin (les adaptateurs ne doivent jamais se connaître entre eux, seulement connaître les ports.)
type Server struct {
	echo          *echo.Echo
	placeRepo     ports.PlaceRepository
	scriptRepo    ports.ScriptRepository
	audioFileRepo ports.AudioFileRepository
	publisher     ports.AudioJobPublisher
	storage       ports.AudioStorage
}

func NewServer(placeRepo ports.PlaceRepository, scriptRepo ports.ScriptRepository, audioFileRepo ports.AudioFileRepository, publisher ports.AudioJobPublisher, storage ports.AudioStorage) *Server {
	s := &Server{
		echo:          echo.New(),
		placeRepo:     placeRepo,
		scriptRepo:    scriptRepo,
		audioFileRepo: audioFileRepo,
		publisher:     publisher,
		storage:       storage,
	}
	s.echo.GET("/places", s.listPlaces)
	s.echo.GET("/places/:id/audio", s.getPlaceAudio)
	s.echo.POST("/scripts/:id/review", s.reviewScript)
	return s
}

func (s *Server) Start(addr string) error {
	return s.echo.Start(addr)
}
