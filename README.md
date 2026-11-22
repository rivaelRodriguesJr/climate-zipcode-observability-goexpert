# Climate ZIP Code — Observabilidade distribuída

Projeto composto por dois serviços Go:

- **Serviço A** (`localhost:8080`): recebe um CEP por `POST /weather` e orquestra a consulta.
- **Serviço B** (`localhost:8081`): consulta o CEP no ViaCEP, obtém a temperatura atual na WeatherAPI e retorna os valores em Celsius, Fahrenheit e Kelvin.

O rastreamento distribuído é instrumentado com **OpenTelemetry**. Os serviços enviam os traces por OTLP/gRPC ao OpenTelemetry Collector, que os encaminha ao **Zipkin**.

## Pré-requisitos

- Docker e Docker Compose instalados.
- Acesso à internet para baixar as imagens e para as consultas externas feitas pelos serviços.

## 1. Subir a aplicação

Na raiz do projeto (diretório que contém `docker-compose.yaml`), execute:

```bash
docker compose up --build
```

O Compose inicia os seguintes componentes:

| Componente | Endereço local |
|---|---|
| Serviço A | http://localhost:8080 |
| Serviço B | http://localhost:8081 |
| Swagger do Serviço A | http://localhost:8080/swagger/index.html |
| Swagger do Serviço B | http://localhost:8081/swagger/index.html |
| Zipkin | http://localhost:9411 |

Aguarde os containers iniciarem antes de enviar a requisição.

> **Chave da WeatherAPI:** o `docker-compose.yaml` define `WEATHER_API_KEY` para os serviços. Para utilizar uma chave própria, substitua o valor configurado no Compose por uma chave válida da WeatherAPI. Evite versionar chaves reais no repositório.

## 2. Realizar uma requisição POST no Serviço A

O Serviço A recebe um objeto JSON com a propriedade `cep`. O CEP deve ser informado como **string com exatamente 8 dígitos**.

### cURL

Execute no terminal:

```bash
curl -i -X POST http://localhost:8080/weather \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{"cep":"01001001"}'
```

Também é possível utilizar outro CEP válido, por exemplo `01001000`.

### Arquivo `api.http` (VS Code / JetBrains)

Na raiz do projeto existe o arquivo `api.http`, que contém a requisição pronta para execução:

```http
POST /weather HTTP/1.1
Host: localhost:8080
Accept: application/json
Content-Type: application/json

{
  "cep": "01001001"
}
```

Abra o arquivo em uma IDE com suporte a arquivos HTTP (por exemplo, a extensão REST Client do VS Code) e clique em **Send Request** acima da requisição.

### Resposta esperada

Em caso de sucesso, o Serviço A responde com HTTP `200 OK` e um JSON com a cidade e as temperaturas:

```json
{
  "city": "São Paulo",
  "temp_C": 25.0,
  "temp_F": 77.0,
  "temp_K": 298.15
}
```

Os valores acima são apenas ilustrativos: a cidade e a temperatura dependem do CEP consultado e dos dados atuais retornados pelos serviços externos.

Possíveis respostas de erro do Serviço A:

| HTTP | Significado |
|---|---|
| `400 Bad Request` | Corpo JSON inválido ou com campos não reconhecidos. |
| `404 Not Found` | CEP não encontrado. |
| `422 Unprocessable Entity` | CEP ausente, fora do formato ou diferente de uma string de 8 dígitos. |
| `500 Internal Server Error` | Falha interna ou erro em um serviço dependente. |

## 3. Visualizar os traces no Zipkin

1. Com os containers em execução, acesse **[http://localhost:9411](http://localhost:9411)**.
2. Envie uma requisição `POST /weather` ao Serviço A conforme a seção anterior.
3. No Zipkin, atualize a tela e clique em **Run Query** para buscar os traces recentes (ou ajuste o intervalo de tempo da busca).
4. Abra um trace para visualizar sua linha do tempo, duração e spans.

A chamada percorre o fluxo:

```text
Cliente
  |
  | POST /weather
  v
Serviço A
  |
  | GET /weather/{cep}
  v
Serviço B
  |
  +--> ViaCEP
  |
  +--> WeatherAPI
```

O Serviço A cria o span `service-a-request` e propaga o contexto de trace na chamada HTTP ao Serviço B. O Serviço B extrai esse contexto e cria o span `service-b-request`. Assim, os spans dos dois serviços ficam associados ao mesmo trace.

No Zipkin, procure os serviços **`service-a`** e **`service-b`**. A visualização do trace permite inspecionar os spans reportados por ambos os serviços.

### Componentes de observabilidade

- **OpenTelemetry SDK:** instrumenta os serviços e exporta os traces via OTLP/gRPC.
- **OpenTelemetry Collector:** recebe os traces na porta `4317` e os encaminha ao Zipkin.
- **Zipkin:** recebe os spans pela API `/api/v2/spans` e disponibiliza a interface web na porta `9411`.

## 4. Encerrar a aplicação

Na raiz do projeto, execute:

```bash
docker compose down
```

Para encerrar e também remover volumes declarados pelo Compose, se houver:

```bash
docker compose down --volumes
```
