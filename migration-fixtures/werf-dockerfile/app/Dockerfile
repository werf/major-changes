FROM alpine:3.20 AS build
WORKDIR /src
COPY . .
RUN cat main.txt > /out.txt

FROM alpine:3.20
COPY --from=build /out.txt /app/out.txt
CMD ["cat", "/app/out.txt"]
