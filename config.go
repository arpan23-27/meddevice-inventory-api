package main 

import "os"

type Config struct {
	Port string
	PostgresURL string
	RedisURL string
}

func LoadConfig()  Config {
	return Config{
		Port:   getenv("PORT",  "8080"),
		PostgresURL: getenv("POSTGRES_URL", "postgres://postgres:postgres@localhost:5432/meddevices?sslmode=disable"),
		RedisURL:  getenv("REDIS_URL",  "localhost:6379"),

	}
}

func getenv(k,  def  string)  string {
	if  v := os.Getenv(k);  v != "" {
		 return v
	}
	return  def
}