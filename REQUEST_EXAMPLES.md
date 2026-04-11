# REQUEST_EXAMPLES

Ниже собраны примеры запросов для работы с API задач.

---

## 1. Create task

### cURL
```bash
curl --location 'http://localhost:8080/api/v1/tasks' \
--header 'Content-Type: application/json' \
--data '{
  "title": "Прием витаминов",
  "description": "Курс на 2 недели",
  "status": "in_progress",
  "scheduled_at": "2026-04-13T09:00:00Z"
}'
```

### Postman
**URL:** `http://localhost:8080/api/v1/tasks`

**Body (raw / JSON):**
```json
{
  "title": "Прием витаминов",
  "description": "Курс на 2 недели",
  "status": "in_progress",
  "scheduled_at": "2026-04-13T09:00:00Z"
}
```

---

## 2. Create tasks in a recurrent series

### cURL
```bash
curl --location 'http://localhost:8080/api/v1/tasks' \
--header 'Content-Type: application/json' \
--data '{
  "title": "Test recurrence",
  "description": "Курс на 2 недели",
  "status": "new",
  "scheduled_at": "2026-04-13T09:00:00Z",
  "recurrence_type": "weekly",
  "recurrence": {
    "week_days": [1, 4]
  }
}'
```

### Postman
**URL:** `http://localhost:8080/api/v1/tasks`

**Body (raw / JSON):**
```json
{
  "title": "Test recurrence",
  "description": "Курс на 2 недели",
  "status": "new",
  "scheduled_at": "2026-04-13T09:00:00Z",
  "recurrence_type": "weekly",
  "recurrence": {
    "week_days": [1, 4]
  }
}
```

---

## 3. Update — single task only

### cURL
```bash
curl --location --request PUT 'http://localhost:8080/api/v1/tasks/5' \
--header 'Content-Type: application/json' \
--data '{
  "title": "Купить протеин (только сегодня со скидкой)",
  "description": "Зайти в магазин на углу, там акция",
  "status": "new",
  "scheduled_at": "2026-07-15T09:00:00Z"
}'
```

### Postman
**URL:** `http://localhost:8080/api/v1/tasks/5`

**Body (raw / JSON):**
```json
{
  "title": "Купить протеин (только сегодня со скидкой)",
  "description": "Зайти в магазин на углу, там акция",
  "status": "new",
  "scheduled_at": "2026-07-15T09:00:00Z"
}
```

---

## 4. Update — bulk content without changing recurrence

### cURL
```bash
curl --location --request PUT 'http://localhost:8080/api/v1/tasks/4' \
--header 'Content-Type: application/json' \
--data '{
  "title": "Регулярная тренировка (обновленный план)",
  "description": "1. Разминка 2. Жим 3. Бассейн",
  "status": "new",
  "apply_to_all": true,
  "recurrence_type": "weekly",
  "recurrence": {
    "week_days": [1, 4]
  }
}'
```

### Postman
**URL:** `http://localhost:8080/api/v1/tasks/4`

**Body (raw / JSON):**
```json
{
  "title": "Регулярная тренировка (обновленный план)",
  "description": "1. Разминка 2. Жим 3. Бассейн",
  "status": "new",
  "apply_to_all": true,
  "recurrence_type": "weekly",
  "recurrence": {
    "week_days": [1, 4]
  }
}
```

---

## 5. Update — rescheduling time for the whole series

### cURL
```bash
curl --location --request PUT 'http://localhost:8080/api/v1/tasks/43' \
--header 'Content-Type: application/json' \
--data '{
  "title": "Регулярная тренировка (обновленный план)",
  "description": "1. Разминка 2. Жим 3. Бассейн",
  "scheduled_at": "2026-04-12T11:00:00Z",
  "apply_to_all": true,
  "status": "done",
  "recurrence_type": "daily",
  "recurrence": {
    "interval": 1
  }
}'
```

### Postman
**URL:** `http://localhost:8080/api/v1/tasks/43`

**Body (raw / JSON):**
```json
{
  "title": "Регулярная тренировка (обновленный план)",
  "description": "1. Разминка 2. Жим 3. Бассейн",
  "scheduled_at": "2026-04-12T11:00:00Z",
  "apply_to_all": true,
  "status": "done",
  "recurrence_type": "daily",
  "recurrence": {
    "interval": 1
  }
}
```

---

## 6. Update — change recurrence logic

### cURL
```bash
curl --location --request PUT 'http://localhost:8080/api/v1/tasks/102' \
--header 'Content-Type: application/json' \
--data '{
  "title": "Регулярная тренировка (обновленный план)",
  "description": "1. Разминка 2. Жим 3. Бассейн",
  "scheduled_at": "2026-04-12T11:00:00Z",
  "apply_to_all": true,
  "status": "done",
  "recurrence_type": "specific",
  "recurrence": {
    "specific_dates": [
      "2026-05-01T09:00:00Z",
      "2026-05-03T09:00:00Z",
      "2026-05-10T09:00:00Z"
    ]
  }
}'
```

### Postman
**URL:** `http://localhost:8080/api/v1/tasks/102`

**Body (raw / JSON):**
```json
{
  "title": "Регулярная тренировка (обновленный план)",
  "description": "1. Разминка 2. Жим 3. Бассейн",
  "scheduled_at": "2026-04-12T11:00:00Z",
  "apply_to_all": true,
  "status": "done",
  "recurrence_type": "specific",
  "recurrence": {
    "specific_dates": [
      "2026-05-01T09:00:00Z",
      "2026-05-03T09:00:00Z",
      "2026-05-10T09:00:00Z"
    ]
  }
}
```

---

## 7. Delete — single task

### cURL
```bash
curl --location --request DELETE 'http://localhost:8080/api/v1/tasks/22?mode=single'
```

### Postman
**URL:** `http://localhost:8080/api/v1/tasks/22?mode=single`

---

## 8. Delete — current and future tasks

### cURL
```bash
curl --location --request DELETE 'http://localhost:8080/api/v1/tasks/43?mode=future'
```

### Postman
**URL:** `http://localhost:8080/api/v1/tasks/43?mode=future`

---

## 9. Delete — entire series

### cURL
```bash
curl --location --request DELETE 'http://localhost:8080/api/v1/tasks/133?mode=entire_series&deleteModified=false'
```

### Postman
**URL:** `http://localhost:8080/api/v1/tasks/133?mode=entire_series&deleteModified=false`

---

## 10. Get task

### cURL
```bash
curl --location 'http://localhost:8080/api/v1/tasks/22'
```

### Postman
**URL:** `http://localhost:8080/api/v1/tasks/22`

---

## 11. Get list of tasks

### cURL
```bash
curl --location 'http://localhost:8080/api/v1/tasks'
```

### Postman
**URL:** `http://localhost:8080/api/v1/tasks`
