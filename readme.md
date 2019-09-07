GOROOT=/usr/lib/go-1.10
GOPATH=/user/local/go

docker:
sudo docker run -e 'ACCEPT_EULA=Y' -e 'SA_PASSWORD=testtest1!' -p 1433:1433 -d mcr.microsoft.com/mssql/server:2017-CU8-ubuntu

sudo docker exec -it f39c9b2056a7 /opt/mssql-tools/bin/sqlcmd -S localhost -U sa -P testtest1!


CREATE DATABASE chat
GO
USE chat


CREATE TABLE users(id int identity(1,1), username varchar(255), password varchar(255), is_male bit, PRIMARY KEY(id), UNIQUE(username));

CREATE TABLE active_users(user_id int, FOREIGN KEY(user_id) REFERENCES users(id), UNIQUE(user_id));

CREATE TABLE messages(id int identity(1,1), from_user_id int, to_user_id int, text varchar(2096), time datetime, PRIMARY KEY(id), FOREIGN KEY(from_user_id) REFERENCES users(id), FOREIGN KEY(to_user_id) REFERENCES users(id));

INSERT INTO users(username, password, is_male) values('broadcast', '', 1);
INSERT INTO users(username, password, is_male) values('test', '$2a$14$nmh9tzLv9DEX8Cpx72/ESO2L9ijODdCYQe1aJuLl2qsZNzKB7/key', 1);