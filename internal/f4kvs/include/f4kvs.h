#ifndef F4KVS_H
#define F4KVS_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef enum {
    F4KVS_SUCCESS = 0,
    F4KVS_ERROR_INVALID_ARGUMENT = 1,
    F4KVS_ERROR_NOT_FOUND = 2,
    F4KVS_ERROR_STORAGE = 3,
    F4KVS_ERROR_NETWORK = 4,
    F4KVS_ERROR_TIMEOUT = 5,
    F4KVS_ERROR_UNKNOWN = 99
} F4KvsResult;

typedef struct F4KvsEngine F4KvsEngine;

typedef struct {
    char *key;
    uint8_t *value;
    size_t value_len;
} F4KvsKVPair;

typedef struct {
    F4KvsKVPair *pairs;
    size_t count;
} F4KvsScanResult;

F4KvsEngine *f4kvs_engine_new(void);
F4KvsEngine *f4kvs_engine_open(const char *data_dir);
F4KvsResult f4kvs_engine_close(F4KvsEngine *engine);
void f4kvs_engine_free(F4KvsEngine *engine);
F4KvsResult f4kvs_engine_compact(F4KvsEngine *engine);

F4KvsResult f4kvs_engine_put(F4KvsEngine *engine, const char *key, const char *value);
F4KvsResult f4kvs_engine_get(F4KvsEngine *engine, const char *key, char **value_out);
F4KvsResult f4kvs_engine_delete(F4KvsEngine *engine, const char *key);
F4KvsResult f4kvs_engine_exists(F4KvsEngine *engine, const char *key, int *exists_out);

F4KvsResult f4kvs_engine_put_bytes(F4KvsEngine *engine, const char *key, const uint8_t *value,
                                   size_t value_len);
F4KvsResult f4kvs_engine_get_bytes(F4KvsEngine *engine, const char *key, uint8_t **value_out,
                                   size_t *value_len_out);
void f4kvs_bytes_free(uint8_t *ptr);

F4KvsResult f4kvs_engine_scan_prefix(F4KvsEngine *engine, const char *prefix,
                                     F4KvsScanResult *result_out);
void f4kvs_scan_result_free(F4KvsScanResult *result);

const char *f4kvs_get_last_error(void);
const char *f4kvs_result_to_string(F4KvsResult result);
void f4kvs_string_free(char *ptr);

#ifdef __cplusplus
}
#endif

#endif /* F4KVS_H */
