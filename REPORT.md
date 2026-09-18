# Estudo de estratégias de rate limiting concorrente em Go

## Resumo

Este projeto começou como um exercício para entender como diferentes algoritmos de rate limiting se comportam, especialmente sob requisições concorrentes. Em vez de utilizar uma biblioteca pronta, implementei Token Bucket, Leaky Bucket e Sliding Window em Go, protegi o estado compartilhado com mutexes e coloquei as políticas atrás de um servidor HTTP.

O objetivo não foi produzir um rate limiter pronto para uso em produção. O foco foi implementar os algoritmos, testar sua correção, criar workloads controlados e observar diferenças de comportamento, contenção e custo. Durante o estudo, também implementei duas interpretações de Leaky Bucket: um **policer**, que aceita ou rejeita imediatamente, e um **shaper**, que mantém uma fila e libera requisições em uma taxa constante.

Os principais resultados foram:

- Token Bucket e Leaky Bucket Policer apresentaram comportamento quase idêntico quando configurados com taxas equivalentes.
- Sliding Window aplicou um limite mais rígido após um burst, sem recuperar capacidade gradualmente dentro da mesma janela.
- O Leaky Bucket Shaper espaçou dez requisições em aproximadamente 100 ms entre liberações, concluindo o lote em cerca de um segundo.
- Um mutex exclusivo usado em todas as consultas ao mapa limitava o paralelismo entre clientes independentes.
- A substituição desse mutex por `sync.RWMutex` reduziu em aproximadamente 29–31% o custo mediano do workload com múltiplas chaves e quatro threads.
- No experimento HTTP, os três policers mantiveram p50 abaixo de 0,7 ms e p99 abaixo de 4,1 ms na máquina utilizada.

## 1. Motivação e pergunta do estudo

Rate limiters costumam ser apresentados por meio de suas regras matemáticas, mas essas regras não contam toda a história. Duas implementações podem aplicar limites parecidos e ainda diferir em como lidam com bursts, no estado armazenado por cliente ou na contenção criada pela sincronização.

A pergunta principal do estudo foi:

> Como diferentes estratégias de rate limiting se comportam sob bursts e acesso concorrente, e quais custos aparecem quando elas são usadas dentro de um servidor HTTP em Go?

O trabalho foi dividido em três partes:

1. **Comportamento:** quais requisições são aceitas, rejeitadas ou adiadas.
2. **Concorrência:** como as implementações se comportam com uma chave compartilhada e com clientes independentes.
3. **Integração:** quanto do custo continua visível quando o limiter é usado em um servidor HTTP real no loopback.

## 2. Implementações

Todas as políticas mantêm estado separado por chave. No servidor de demonstração, a chave é o endereço IP do cliente.

### 2.1 Token Bucket

Cada cliente possui uma quantidade de tokens limitada pela capacidade do bucket. Uma requisição aceita consome um token, e novos tokens são adicionados de acordo com o tempo transcorrido.

Essa política permite bursts até a capacidade acumulada. Quando o bucket fica vazio, novas requisições recebem uma duração de `Retry-After` até o próximo token.

Estado por cliente:

- tokens disponíveis;
- instante da última reposição;
- mutex individual.

### 2.2 Leaky Bucket Policer

O policer representa o backlog como um nível de água. Cada requisição aumenta esse nível em uma unidade, enquanto o tempo transcorrido o reduz continuamente de acordo com a taxa de vazamento. A requisição é rejeitada quando adicionar uma unidade ultrapassaria a capacidade.

Na configuração utilizada, ele possui uma semântica muito próxima do Token Bucket: ambos permitem um burst inicial e recuperam aproximadamente uma unidade de capacidade por intervalo. Essa semelhança foi observada diretamente no primeiro experimento.

### 2.3 Sliding Window Log

O Sliding Window mantém os timestamps das requisições aceitas em uma deque circular de tamanho fixo. Antes de tomar uma decisão, remove da frente todos os timestamps que já saíram da janela.

Uma requisição é aceita quando ainda existem menos de `Limit` timestamps dentro da janela. Caso contrário, o timestamp mais antigo determina o `Retry-After`.

Essa implementação fornece uma interpretação direta de “no máximo N requisições nos últimos T segundos”, mas mantém até `N` timestamps por cliente.

### 2.4 Leaky Bucket Shaper

O shaper possui um contrato diferente dos policers. Em vez de responder imediatamente com aceitação ou rejeição, ele coloca a requisição em uma fila bufferizada e bloqueia sua goroutine até que um worker a libere.

Cada chave possui:

- um channel bufferizado;
- um worker;
- um ticker configurado com o intervalo entre liberações.

Em cada tick, o worker libera no máximo uma requisição ativa. Requisições com contexto cancelado são descartadas, e uma tentativa de entrada em uma fila cheia retorna erro imediatamente.

Essa variante demonstra suavização de tráfego, mas adiciona latência e mantém uma goroutine por chave. Por isso ela foi analisada separadamente dos microbenchmarks de `Allow()`.

## 3. Sincronização

Cada bucket ou janela possui seu próprio `sync.Mutex`, que protege apenas o estado de um cliente. O mapa que associa chaves a estados é protegido separadamente.

Na primeira versão, até consultas a entradas existentes adquiriam um `sync.Mutex` exclusivo no mapa:

```text
requisição → mutex global do mapa → mutex individual do cliente
```

O benchmark concorrente mostrou que clientes independentes ainda eram parcialmente serializados nessa consulta. A implementação foi então alterada para utilizar `sync.RWMutex`:

- `RLock()` para consultar entradas existentes;
- `Lock()` somente para criar novas entradas;
- uma segunda consulta após adquirir o lock exclusivo, evitando que duas goroutines criem estados diferentes para a mesma chave.

O mutex individual continua necessário. Assim, requisições para a mesma chave permanecem serializadas, enquanto chaves diferentes conseguem avançar em paralelo depois da consulta ao mapa.

## 4. Metodologia

### 4.1 Ambiente

- Sistema operacional: Linux
- Arquitetura: amd64
- Processador: Intel Core i3-7020U @ 2.30 GHz
- Benchmark de concorrência: `GOMAXPROCS` em 1, 2 e 4
- Servidor HTTP: loopback local usando `net/http`

A versão de Go não foi registrada junto aos primeiros resultados. Esse é um dado que deve ser incluído em uma futura repetição do estudo.

### 4.2 Validação de correção

Antes dos experimentos, foram criados testes para verificar:

- burst até a capacidade;
- rejeição após o limite;
- `Retry-After` positivo;
- recuperação com o tempo;
- estado independente entre chaves;
- respeito ao limite sob chamadas concorrentes;
- espaçamento do shaper;
- fila cheia e cancelamento de contexto.

Os testes foram repetidos e executados com o race detector:

```bash
go test -count=10 ./...
go test -race -count=5 ./...
go vet ./...
```

### 4.3 Estatísticas utilizadas

Os benchmarks de desempenho foram repetidos cinco vezes. Para resumir os resultados, foi utilizada a **mediana**, pois ela reduz a influência de execuções isoladas afetadas pelo scheduler, frequência da CPU ou outros processos da máquina.

Também são apresentados intervalos observados quando relevantes. Não foram realizados testes de significância estatística. Portanto, diferenças pequenas são tratadas como ruído possível, não como prova de que um algoritmo é universalmente superior.

No experimento HTTP, p50, p95 e p99 foram calculados com o método nearest-rank sobre as latências individuais das 5.000 requisições de cada execução. A mediana das cinco execuções foi utilizada na tabela final.

## 5. Experimento 1 — burst e recuperação

### Pergunta

Como cada policer reage quando recebe mais requisições que sua capacidade, e como recupera a capacidade depois do burst?

### Configuração

| Política | Configuração |
|---|---|
| Token Bucket | capacidade 10; um token a cada 100 ms |
| Leaky Bucket Policer | capacidade 10; vazamento de 10 unidades/s |
| Sliding Window | 10 requisições em uma janela de 1 s |

O workload enviou 20 requisições imediatamente e, em seguida, uma requisição a cada 100 ms durante 1,2 segundo.

### Hipótese

- Token Bucket deveria aceitar dez requisições imediatamente e recuperar um token a cada 100 ms.
- Leaky Bucket Policer deveria apresentar recuperação gradual semelhante.
- Sliding Window deveria manter o burst bloqueado até os timestamps iniciais saírem da janela.

### Resultado

| Política | Burst inicial | Primeiro `Retry-After` | Recuperação observada |
|---|---:|---:|---|
| Token Bucket | 10 aceitas / 10 rejeitadas | ~99 ms | uma aceitação a cada ~100 ms |
| Leaky Bucket Policer | 10 aceitas / 10 rejeitadas | ~99 ms | uma aceitação a cada ~100 ms |
| Sliding Window | 10 aceitas / 10 rejeitadas | ~999 ms | rejeitou até ~1.003 ms |

Os valores de 99 ms e 999 ms são consequência do pequeno tempo transcorrido entre a atualização do estado e a conversão da duração para milissegundos.

### Interpretação

Token Bucket e Leaky Bucket Policer foram praticamente indistinguíveis nesse workload. Ambos recuperaram capacidade de forma gradual e aceitaram as sondagens espaçadas em 100 ms.

Sliding Window foi mais rígido: nenhuma capacidade foi recuperada antes que as requisições do burst completassem um segundo. Quando os timestamps iniciais expiraram, a política voltou a aceitar.

Esse resultado motivou a implementação do Leaky Bucket Shaper. O policer estava correto, mas não demonstrava a propriedade de suavização normalmente associada ao nome Leaky Bucket.

## 6. Experimento 2 — suavização pelo shaper

### Pergunta

Se dez requisições chegarem simultaneamente, o shaper consegue liberá-las em intervalos aproximadamente constantes?

### Configuração

- capacidade da fila: 10;
- intervalo de vazamento: 100 ms;
- requisições concorrentes: 10.

### Resultado

| Posição de saída | Conclusão acumulada | Intervalo desde a anterior |
|---:|---:|---:|
| 1 | 100 ms | 100 ms |
| 2 | 200 ms | 100 ms |
| 3 | 300 ms | 100 ms |
| 4 | 401 ms | 100 ms |
| 5 | 500 ms | 99 ms |
| 6 | 600 ms | 100 ms |
| 7 | 700 ms | 100 ms |
| 8 | 801 ms | 100 ms |
| 9 | 900 ms | 99 ms |
| 10 | 1.000 ms | 100 ms |

### Interpretação

O shaper liberou uma requisição por intervalo e distribuiu o lote por aproximadamente um segundo. As variações de 99–100 ms são compatíveis com arredondamento e agendamento do sistema operacional.

Os IDs das requisições não saíram em ordem numérica, pois goroutines iniciadas quase simultaneamente não possuem uma ordem determinística de entrada no channel. O resultado importante é o espaçamento temporal das liberações.

## 7. Experimento 3 — custo sequencial de rejeição

### Pergunta

Quanto custa executar `Allow()` quando um cliente já esgotou sua capacidade e continua enviando requisições?

### Método

Cada limiter foi preenchido antes do início da medição. Durante o trecho cronometrado, todas as chamadas usaram a mesma chave e seguiram o caminho de rejeição. O setup não foi incluído no tempo medido.

### Resultado

| Política | Mediana | Intervalo observado | Alocações |
|---|---:|---:|---:|
| Token Bucket | 182,1 ns/op | 176,6–190,5 ns/op | 0 allocs/op |
| Leaky Bucket Policer | 181,8 ns/op | 179,9–182,6 ns/op | 0 allocs/op |
| Sliding Window | 155,6 ns/op | 145,3–160,5 ns/op | 0 allocs/op |

### Interpretação

Sliding Window apresentou o menor custo nesse cenário específico. Como nenhum timestamp expirava, o algoritmo apenas consultava o timestamp da frente, verificava que a deque estava cheia e calculava o tempo restante. Token Bucket e Leaky Bucket recalculavam reposição ou vazamento usando o tempo transcorrido.

Esse resultado não significa que Sliding Window seja sempre mais rápido. O experimento não mediu inserção, remoção de vários timestamps expirados, criação de clientes ou uso de memória persistente. Ele representa apenas o caminho de rejeição de uma chave saturada.

As três implementações apresentaram zero alocações por operação porque todo o estado foi criado antes de `b.ResetTimer()` e reutilizado durante a medição.

## 8. Experimento 4 — contenção e otimização do mapa

### Pergunta

Como os limiters se comportam quando várias goroutines disputam a mesma chave, em comparação com goroutines que acessam chaves independentes?

### Método

Foram utilizados dois cenários:

- `same_key`: todas as goroutines acessam um único cliente saturado;
- `many_keys`: cada worker utiliza uma chave previamente criada e saturada.

O benchmark foi executado com 1, 2 e 4 threads e repetido cinco vezes.

### Resultado inicial

Com quatro threads, múltiplas chaves foram mais rápidas que uma única chave, mas não escalaram de forma consistente com o aumento de threads. A inspeção da implementação mostrou que toda consulta ao mapa ainda adquiria um mutex exclusivo.

### Alteração

O `sync.Mutex` do mapa foi substituído por `sync.RWMutex`, mantendo locks exclusivos apenas durante a criação de novas entradas. Os mutexes individuais dos buckets não foram removidos.

### Antes e depois com quatro threads

| Política | Workload | Mutex | RWMutex | Mudança em ns/op |
|---|---|---:|---:|---:|
| Token Bucket | mesma chave | 284,1 | 282,1 | -0,7% |
| Token Bucket | várias chaves | 216,9 | 152,4 | **-29,7%** |
| Leaky Bucket | mesma chave | 260,4 | 283,8 | +9,0% |
| Leaky Bucket | várias chaves | 195,2 | 138,2 | **-29,2%** |
| Sliding Window | mesma chave | 242,7 | 233,4 | -3,8% |
| Sliding Window | várias chaves | 176,6 | 121,7 | **-31,1%** |

Em throughput, a melhoria do workload multicliente foi de aproximadamente 1,42× para Token Bucket, 1,41× para Leaky Bucket e 1,45× para Sliding Window.

### Resultados após a alteração

| Política | Workload | 1 thread | 2 threads | 4 threads |
|---|---|---:|---:|---:|
| Token Bucket | mesma chave | 199,4 ns | 256,2 ns | 282,1 ns |
| Token Bucket | várias chaves | 194,7 ns | 200,0 ns | 152,4 ns |
| Leaky Bucket | mesma chave | 198,8 ns | 245,8 ns | 283,8 ns |
| Leaky Bucket | várias chaves | 200,4 ns | 165,4 ns | 138,2 ns |
| Sliding Window | mesma chave | 147,7 ns | 188,5 ns | 233,4 ns |
| Sliding Window | várias chaves | 158,5 ns | 119,2 ns | 121,7 ns |

### Interpretação

O `RWMutex` ajudou principalmente no cenário para o qual foi escolhido: várias goroutines consultando entradas existentes e atualizando buckets diferentes. Nesse workload, as três implementações reduziram o custo mediano em aproximadamente 30% com quatro threads.

Na mesma chave, o mutex individual do bucket continuou sendo o gargalo. O `RWMutex` não trouxe uma melhoria consistente e chegou a aumentar o custo do Leaky Bucket. Esse é um trade-off aceitável para o objetivo do projeto, pois o workload esperado possui múltiplos clientes e é predominantemente de leitura no mapa.

Os resultados também não apresentaram escalabilidade linear. A máquina possui poucos núcleos, e operações como `time.Now()`, locks individuais, cache e scheduler continuam tendo custo. Algumas execuções apresentaram variação considerável; por isso as conclusões são baseadas nas medianas e nas tendências repetidas entre as três implementações.

## 9. Experimento 5 — servidor HTTP end-to-end

### Pergunta

As diferenças encontradas nos microbenchmarks continuam relevantes quando o limiter é executado dentro de um servidor HTTP?

### Configuração

- 5.000 requisições por execução;
- concorrência 32;
- uma chave/IP local;
- capacidade 2.500;
- 50% das requisições aceitas e 50% rejeitadas;
- cinco execuções por caso;
- baseline com o mesmo handler, mas sem limiter.

O servidor foi executado no loopback local. As conexões utilizaram keep-alive e foram limitadas ao nível de concorrência do workload.

### Resultado

| Caso | Throughput mediano | p50 | p95 | p99 | Status |
|---|---:|---:|---:|---:|---|
| Baseline | 28.999 req/s | 601 µs | 1.863 µs | 3.095 µs | 5.000 × 200 |
| Token Bucket | 27.142 req/s | 626 µs | 2.072 µs | 3.801 µs | 2.500 × 200; 2.500 × 429 |
| Leaky Bucket Policer | 24.915 req/s | 672 µs | 2.398 µs | 4.085 µs | 2.500 × 200; 2.500 × 429 |
| Sliding Window | 27.726 req/s | 639 µs | 2.032 µs | 3.298 µs | 2.500 × 200; 2.500 × 429 |

Nenhuma das 100.000 requisições executadas nas cinco repetições de cada caso resultou em erro do cliente.

Em relação ao baseline, o throughput mediano foi aproximadamente 6,4% menor para Token Bucket, 14,1% menor para Leaky Bucket e 4,4% menor para Sliding Window. O aumento da latência mediana ficou entre 25 e 71 µs.

### Interpretação

As diferenças de dezenas de nanossegundos observadas nos microbenchmarks ficaram menos importantes dentro do servidor HTTP. Conexão TCP, parsing HTTP, escrita da resposta, transporte no loopback, scheduler e gerenciamento das conexões passaram a representar grande parte do custo.

Sliding Window obteve o maior throughput mediano entre os limiters nesse workload, enquanto Leaky Bucket apresentou menor throughput e maior variação. As distribuições de algumas execuções se sobrepõem, portanto esses números são tratados como observações da máquina e configuração utilizadas, não como um ranking universal.

O resultado mais relevante é que os três policers mantiveram p50 abaixo de 0,7 ms e p99 abaixo de 4,1 ms no ambiente local, mesmo com 32 requisições concorrentes e metade do tráfego sendo rejeitada.

## 10. Comparação prática

| Estratégia | Vantagem principal | Custo ou limitação | Quando eu consideraria usar |
|---|---|---|---|
| Token Bucket | simples, estado constante e permite bursts controlados | não aplica um limite perfeitamente uniforme em toda janela | APIs que permitem pequenos bursts |
| Leaky Bucket Policer | modelo contínuo de recuperação | nesta implementação, oferece pouco comportamento novo em relação ao Token Bucket | quando a política já é descrita naturalmente como nível ou backlog |
| Sliding Window Log | aplica diretamente o limite sobre os últimos `T` segundos | armazena até `N` timestamps por cliente | limites mais estritos e valores moderados de `N` |
| Leaky Bucket Shaper | suaviza a chegada ao processamento downstream | adiciona espera, fila e uma goroutine por chave nesta versão | proteção de consumidores que precisam de fluxo regular |

Não existe um vencedor único. A escolha depende principalmente da semântica desejada. A diferença de custo entre os policers foi pequena quando comparada ao stack HTTP, enquanto suas respostas a bursts foram visivelmente diferentes.

## 11. Limitações

Este estudo possui limitações importantes:

- Os experimentos foram executados em uma única máquina de baixo número de núcleos.
- O tráfego foi sintético e local, sem latência de rede, TLS ou proxy reverso.
- O experimento HTTP utilizou uma única identidade/IP e um handler quase vazio.
- Os resultados antes e depois do `RWMutex` foram medidos em rodadas separadas, não intercaladas.
- Foram utilizadas cinco repetições e medianas, sem análise estatística inferencial.
- Os limiters não compartilham estado entre processos ou máquinas.
- Não existe persistência ou coordenação distribuída.
- Entradas de clientes inativos não são removidas dos mapas.
- O shaper mantém uma goroutine por chave e não encerra workers inativos.
- Requisições canceladas permanecem na fila do shaper até serem descartadas por um tick futuro.
- Não foram avaliados pesos diferentes por requisição, fairness entre clientes ou relógio monotônico injetável para testes determinísticos.
- O benchmark sequencial mediu apenas o caminho saturado de rejeição.
- Os números de throughput representam esta máquina e este workload, não uma capacidade prometida para produção.

Esses pontos não impedem o uso do projeto como estudo, mas delimitam o que pode ser concluído a partir dos resultados.

## 12. Reprodução

### Testes

```bash
go test -count=10 ./...
go test -race -count=5 ./...
go vet ./...
```

### Burst e recuperação

```bash
go run ./cmd/burst-experiment > results/burst.csv
```

### Shaper

```bash
go run ./cmd/shaper-experiment > results/shaper.csv
```

### Custo sequencial

```bash
go test ./policies \
  -run '^$' \
  -bench '^BenchmarkSaturatedSequential$' \
  -benchmem \
  -count=5 \
  | tee results/sequential.txt
```

### Concorrência

```bash
go test ./policies \
  -run '^$' \
  -bench '^BenchmarkSaturatedConcurrent$' \
  -benchmem \
  -cpu=1,2,4 \
  -count=5 \
  | tee results/concurrent-after-rwmutex.txt
```

### HTTP end-to-end

```bash
go run ./cmd/http-experiment > results/http.csv
```

Arquivos de resultados esperados:

```text
results/
├── burst.csv
├── shaper.csv
├── sequential.txt
├── concurrent-before-rwmutex.txt
├── concurrent-after-rwmutex.txt
└── http.csv
```

## 13. Conclusão

O projeto começou como uma implementação de três algoritmos conhecidos, mas a parte mais útil foi observar onde minhas expectativas estavam incompletas.

Eu esperava que Leaky Bucket apresentasse um comportamento claramente diferente de Token Bucket, mas a variante policer ficou quase idêntica no workload de burst. Isso levou à implementação do shaper e tornou concreta a diferença entre rejeitar tráfego e suavizar seu processamento.

Também esperava que mutexes individuais por cliente fossem suficientes para permitir paralelismo. Os benchmarks mostraram que o mutex exclusivo usado apenas para consultar o mapa ainda limitava o workload multicliente. Depois de trocar essa consulta por `RWMutex`, medi uma redução de aproximadamente 30% no custo com quatro threads e chaves independentes.

Finalmente, o experimento HTTP mostrou que otimizações visíveis em nanossegundos não necessariamente produzem diferenças da mesma proporção em uma aplicação completa. O custo do protocolo e do servidor reduziu a distância entre os algoritmos.

O principal aprendizado não foi que um algoritmo é sempre melhor. Foi aprender a separar semântica, implementação e ambiente de execução; formular uma hipótese; medir; encontrar uma limitação; e conferir se uma mudança realmente melhorou o cenário para o qual foi criada.
