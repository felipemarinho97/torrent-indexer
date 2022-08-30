# vercel-lambdas

This is a collection of lambdas for personal use powered by [vercel](https://vercel.com/).

## statusinvest

The path `statusinvest` contains lambdas that fetches the status of a company (fundamentalist analysis) from [statusinvest.com](https://statusinvest.com/).

## b3

The path `b3` contains lambdas that fetches stock prices data directly from from [b3.com](https://b3.com/).

- `b3/prices/:symbol` [Test it!](https://vercel-lambdas-felipemarinho.vercel.app/api/b3/prices/IBOV)
    - Returns the stock prices for a given symbol.
    - Example Response:
    ```json
    {
        "symbol": "IBOV",
        "name": "IBOVESPA",
        "market": "Indices",
        "openingPrice": 112295.87,
        "minPrice": 111689.15,
        "maxPrice": 113221.54,
        "averagePrice": 112702.99,
        "currentPrice": 112323.12,
        "priceVariation": 0.02,
        "indexComponentIndicator": false
    }
    ```
