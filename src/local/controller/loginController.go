package contoller

import (
	routing "github.com/go-ozzo/ozzo-routing/v2"
	_ "github.com/go-sql-driver/mysql"
	"github.com/go-ozzo/ozzo-dbx"
	"pkg/dbcontext"
	"pkg/log"
	"encoding/json"
)

type requestData struct{
	LoginName string `json:"loginname"`
	Password string `json:"password"`
}

type responseData struct{
	Id int `json:"id"`
	Department string `json:"department"`
	Purview string `json:"purview"`
	Logname string `json:"logname"`
}

type DB_Login struct {
	Id int `db:"id"`
	Department string `db:"department"`
	Purview string `db:"purview"`
	Logname string `db:"logname"`
}

type ErrorResponseData struct{
	Error string `json:"error"`
}

func RegisterLoginHandlers(rg *routing.RouteGroup, logger log.Logger, db *dbcontext.DB) {
	rg.Post("/login", loginHandler(logger, db))
}

func loginHandler(logger log.Logger, db *dbcontext.DB) routing.Handler {
	return func(c *routing.Context) error {
		rd := requestData{}
		if err := c.Read(&rd); err != nil {
			logger.With(c.Request.Context()).Errorf("invalid request: %v", err)
			return err
		}

		q := db.DB().Select("id", "department", "purview", "logname").
			From("loguser").
			Where(dbx.HashExp{"logname": rd.LoginName, "logpassword": rd.Password}).
			OrderBy("id")

		var users [] DB_Login
		err := q.All(&users)
		if err != nil {
			logger.With(c.Request.Context()).Errorf("database query error: %v", err)
			return err
		}

		var usersNum = len(users)
		if usersNum <= 0 {
			logger.With(c.Request.Context()).Errorf("database query error: %v", err)
			rp := &ErrorResponseData{}
			rp.Error = "Loginname or password not correct."
			b, err := json.Marshal(rp)
			if err != nil {
				logger.With(c.Request.Context()).Errorf("response format to json error: %v", err)
				return err
			}
			return c.Write(string(b))
		}

		rp := &responseData{}
		rp.Id = users[0].Id
		rp.Department = users[0].Department
		rp.Purview = users[0].Purview
		rp.Logname = users[0].Logname
		b, err := json.Marshal(rp)
		if err != nil {
			logger.With(c.Request.Context()).Errorf("response format to json error: %v", err)
			return err
		}
		return c.Write(string(b))
    }
}