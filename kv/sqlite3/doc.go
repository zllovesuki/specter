// Initial implementation was assisted by Deepseek R1 14B running locally and OpenAI GPT-4o mini using the following prompt:
// {{prompt}}
// I need to write a small golang library that uses sqlite3 to expose an KV interface.
//
// Basic requirements:
// 1. The key for each entry is an arbitrary binary blob;
// 2. Each KV entry supports 3 types of values: Simple, Prefix, and Lease;
// 3. Simple value stores an arbitrary binary blob, and be updated on key conflict;
// 4. Prefix value stores an array of strings. The values need to be unique within each entry. However, different keys may have the same values in Prefix;
// 5. Lease value stores an uint64. This also needs to support an atomic update with the business logic done in go rather than SQL;
// 6. Each KV entry may have all three types value at the same time. For example, operating on a Simple value should not affect the values in Prefix nor Lease;
// 7. Read operations should return zero values and should not error when the corresponding value type is not found;
// 8. If inserting a value that already exists in the Prefix within each key, returns an error;
// 9. In addition to support three types of values for each key, each key will need to have a KeyTracker, where it will mark what value types this key has. KeyTracker also stores the hash of the key via a custom function that outputs uint64;
// 10. Multiple keys may have a same hash value in KeyTracker due to the hash function used. It is important to only calculate and store the hash when the KeyTracker was first created.
// 11. When there are no longer Simple, Prefix, or Lease values under a key, the KeyTracker will also need to be deleted;
// 12. Avoid using AUTO_INCREMENT and use composite keys when possible.
//
// Additional requirements:
// 1. The KV interface also expects functions to export and restore KV entries. This is needed for backup and restore.
// 2. The library needs to be concurrency safe. Avoid using mutex in go, and use transaction where possible.
//
// Please implement the library with the specified requirements. If possible, please the GORM ORM library.
// {{/prompt}}
//
// Some tests and optimizations were assisted by OpenAI GPT-4o mini.
package sqlite3
