# Build stage
FROM golang:1.27 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /out/app .

FROM scratch
ENV PORT=8080
COPY --from=build /out/app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
