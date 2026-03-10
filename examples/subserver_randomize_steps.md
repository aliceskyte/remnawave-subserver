# Пример `subserver.randomize`

Файл [subserver_randomize.json](/opt/subserver/examples/subserver_randomize.json)
показывает Xray-шаблон, в котором alias-tag `primary` выбирается из двух
candidate-outbound:

```json
"subserver": {
  "randomize": {
    "primary": ["primary_a", "primary_b"]
  }
}
```

## Что происходит по шагам

1. `Remnawave Subserver` читает блок `subserver.randomize`.
2. Для группы `primary` он видит двух кандидатов: `primary_a` и `primary_b`.
3. На каждом запросе подписки случайно выбирается один из них.
4. Выбранный outbound остается в итоговом конфиге, но его `tag` становится `primary`.
5. Второй candidate-outbound удаляется из итогового Xray JSON.
6. Блок `subserver` целиком удаляется и клиент его не получает.
7. `routing.balancers[].selector`, где уже указан `primary`, продолжает работать с
   выбранным outbound под итоговым tag `primary`.

## Что увидит клиент

На выходе у клиента никогда не будет одновременно `primary_a` и `primary_b`.
Будет только один outbound с tag `primary`.

### Вариант 1

Если выбран `primary_a`, клиент получит outbound примерно такого вида:

```json
{
  "tag": "primary",
  "protocol": "vless",
  "settings": {
    "vnext": [
      {
        "address": "primary-a.example.com"
      }
    ]
  }
}
```

### Вариант 2

Если выбран `primary_b`, клиент получит outbound примерно такого вида:

```json
{
  "tag": "primary",
  "protocol": "vless",
  "settings": {
    "vnext": [
      {
        "address": "primary-b.example.com"
      }
    ]
  }
}
```

## Что остается без изменений

- `primary_via_gateway` и `primary_fallback` остаются как есть.
- `secondary`, `secondary_via_gateway`, `secondary_fallback`, `direct`, `block` остаются как есть.
- Логика роутинга по `balancerTag` не меняется.
- `burstObservatory.subjectSelector` продолжает видеть `primary` как итоговый tag.
