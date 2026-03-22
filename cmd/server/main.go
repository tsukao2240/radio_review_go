package main

import (
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, using environment variables")
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("radio_review_go: ok"))
	})

	// TODO: フェーズ3でハンドラーを登録する
	// r.Mount("/", handler.NewBroadcastRouter(broadcastHandler))
	// r.Mount("/recording", handler.NewRecordingRouter(recordingHandler))
	// r.Mount("/review", handler.NewPostRouter(postHandler))
	// r.Mount("/favorites", handler.NewFavoriteRouter(favoriteHandler))

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("server starting on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
