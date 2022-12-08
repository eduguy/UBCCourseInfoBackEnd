package main

import (
	"context"
	"database/sql"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	_ "github.com/jackc/pgx/v4/stdlib"
)

var globalDb *sql.DB

func updateDb(firstName string, lastName string, rating float64) {
	row := globalDb.QueryRowContext(context.Background(), "INSERT into ratings VALUES($1,$2,$3) ON CONFLICT (firstName, lastName) DO UPDATE SET date = EXCLUDED.date;", firstName, lastName, rating)
	err := row.Scan()
	if err != nil {
		log.Println("Insertion failed: ", err)
	}
}

func getRating(c *gin.Context) {
	scraperChannel := make(chan float64)
	dbCacheChannel := make(chan float64)
	dbLastCheckedChannel := make(chan time.Time)

	firstName := c.Query("firstName")
	lastName := c.Query("lastName")

	go func() {
		resp, err := http.Get("http://" + os.Getenv("SCRAPER") + "/rate?firstName=" + firstName + "&lastName=" + lastName)

		if err != nil {
			log.Println("Failed to query scraper", err)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Println("Failed to parse response body: ", err)
		}

		sb := string(body)
		rating, err := strconv.ParseFloat(sb, 64)
		if err != nil {
			log.Println("Failed to parse string to float: ", err)
		}
		scraperChannel <- rating
	}()

	go func() {

		row := globalDb.QueryRowContext(context.Background(), "SELECT rating, date from ratings WHERE firstName = $1 AND lastName = $2", firstName, lastName)
		var rating float64
		var date string

		if err := row.Scan(&rating, &date); err != nil {
			log.Println("row.Scan", err)
			close(dbCacheChannel)
			close(dbLastCheckedChannel)
			return
		}

		dbCacheChannel <- rating
		dbLastChecked, err := time.Parse(time.RFC3339Nano, date)
		if err != nil {
			log.Println("time.Parse", err)
			close(dbCacheChannel)
			close(dbLastCheckedChannel)
			return
		}
		dbLastCheckedChannel <- dbLastChecked

	}()

	dbCacheResult := <-dbCacheChannel
	dbLastCheckedResult := <-dbLastCheckedChannel

	if !dbLastCheckedResult.AddDate(0, 0, 5).Before(time.Now()) {
		c.JSON(http.StatusOK, dbCacheResult)
		go func() {
			scrapeResult := <-scraperChannel
			updateDb(firstName, lastName, scrapeResult)
		}()
		return
	}

	scrapeResult := <-scraperChannel
	c.JSON(http.StatusOK, scrapeResult)
	go func() {
		updateDb(firstName, lastName, scrapeResult)
	}()

}

func main() {

	err := godotenv.Load(".env")

	if err != nil {
		log.Fatal("Error loading .env file")
	}

	dsn := url.URL{
		Host:   os.Getenv("GOHOST") + ":" + os.Getenv("GOPORT"),
		Path:   os.Getenv("GODBPATH"),
		User:   url.UserPassword(os.Getenv("GODBUSERNAME"), os.Getenv("GODBPASSWORD")),
		Scheme: os.Getenv("GOSCHEME"),
	}

	q := dsn.Query()
	q.Add("sslmode", "disable")

	dsn.RawQuery = q.Encode()

	db, err := sql.Open("pgx", dsn.String())

	if err != nil {
		log.Fatal("Connection failed to open", err)
	}

	defer func() {
		db.Close()
	}()

	if err := db.PingContext(context.Background()); err != nil {
		log.Fatal("DB failed to connect: ", err)
	}
	globalDb = db
	router := gin.Default()
	router.Use(cors.Default())
	router.GET("/rate", getRating)
	router.Run("localhost:5001")
}
