# SQLite to MySQL Migration

The archive service can use SQLite or MySQL 9.x. SQLite remains the zero-service default. MySQL is intended for deployments that want a separately managed database, larger connection concurrency, and online database backup tooling.

## Safe cutover

1. Stop every DahuaBridge instance that can write the SQLite archive. This is required for a stable source snapshot.
2. Back up `archive.db` together with any `archive.db-wal` and `archive.db-shm` files before changing configuration.
3. Create an empty MySQL database and a dedicated application user. Grant that user only `SELECT`, `INSERT`, `UPDATE`, `DELETE`, `CREATE`, `ALTER`, and `INDEX` on that database. Startup never drops archive tables.
4. Mount the password as a read-only secret and select MySQL in `config.yaml`:

```yaml
archive:
  enabled: true
  db_driver: mysql
  db_host: mysql
  db_port: 3306
  db_name: dahuabridge
  db_user: dahuabridge
  db_password_file: /run/secrets/dahuabridge-db-password
  db_tls: true
  db_connect_timeout: 1m
  db_max_open_conns: 8
  db_max_idle_conns: 4
  db_conn_max_lifetime: 30m
  db_conn_max_idle_time: 5m
  temp_dir: /data/archive/tmp
```

5. With the normal bridge service still stopped, run the one-shot importer using the same image, mounts, network, and config:

```text
/app/dahuabridge --config /config/config.yaml migrate-archive --source-sqlite /data/archive/archive.db
```

The source is opened with SQLite `mode=ro` and `query_only=1`; the importer never changes its tables. Stop every writer before copying or importing it. If `archive.db-wal` or `archive.db-shm` exists, preserve and mount those files together with `archive.db`: committed rows may exist only in the WAL. SQLite must either receive the complete readable WAL/SHM pair or have write access to their directory for lock and shared-memory bookkeeping. Never delete sidecars merely to make the importer start.

After a clean shutdown and checkpoint, SQLite normally removes the WAL/SHM sidecars while retaining WAL mode in the database header. When no WAL, SHM, or rollback-journal sidecar exists, the importer opens that stable snapshot with SQLite `immutable=1`. This safely supports a read-only source mount without creating lock or SHM files. The importer deliberately does not enable immutable mode when any sidecar exists, because immutable SQLite would ignore later file changes and could omit committed WAL-only rows.

The importer includes committed WAL contents, initializes the MySQL schema, requires an empty first-use target, copies all five data tables in one serializable transaction, and compares exact row counts plus canonical SHA-256 digests of every field before committing. The same transaction records a source-schema/content fingerprint in MySQL. A rerun with the identical SQLite snapshot is a read-only success and cannot overwrite newer MySQL rows; a changed source is rejected.

Use verified `db_tls: true` whenever the MySQL certificate matches its service name. `skip-verify` is suitable only as a transitional option on a trusted private network when an automatically generated certificate lacks the required SAN; configure the MySQL application account with `REQUIRE SSL`. Do not use `false` outside an explicitly accepted plaintext deployment.

Treat the MySQL database and every backup as secret-bearing data. Archive rows retain RTSP URLs, which can contain camera credentials. Restrict database and backup access to the bridge and its operators, use encrypted storage and encrypted backup transport, keep dumps out of source control, and apply the same retention and secure-deletion policy to backups as to other credential stores. TLS protects data in transit; it does not protect database files or exported backups at rest.

6. Start DahuaBridge and verify `/readyz`, archive searches, SMD/IVS counts, and clip metadata.

On success, the import command prints only table names and verified row counts. It never prints the DSN, password, event data, paths, or device identifiers. Diagnostic errors can include the configured source path, so handle migration logs as operational data.

## Rollback

Stop the bridge, restore `db_driver: sqlite` and the original `db_path`, then restart. The importer never modifies or deletes the SQLite source.

Do not run SQLite and MySQL-backed bridge instances concurrently during cutover: they are independent stores and cannot replicate changes between each other.

Run only one active DahuaBridge writer against an archive database. MySQL provides a managed shared storage service and higher connection concurrency; it does not turn archive jobs and checkpoints into an active-active coordination protocol.
