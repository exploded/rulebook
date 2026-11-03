@echo off
setlocal enabledelayedexpansion

echo Building rulebook...
go build -o rulebook.exe ./cmd/rulebook/
if errorlevel 1 (
    echo Build failed!
    exit /b 1
)

if not exist .env (
    if exist .env.example (
        copy .env.example .env
        echo Created .env from .env.example
    )
)

if exist .env (
    for /f "usebackq tokens=1,* delims==" %%a in (.env) do (
        echo %%a | findstr /r "^#" >nul
        if errorlevel 1 (
            if not "%%a"=="" (
                set "%%a=%%b"
            )
        )
    )
)

if not exist rulebook.db (
    echo rulebook.db not found. Run: sh reload.sh  (loads the Constitution and referendums)
    exit /b 1
)

echo Starting rulebook on port %PORT%...
rulebook.exe
