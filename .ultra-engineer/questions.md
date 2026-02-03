1. How should file logging be enabled?
   A. Command-line flag only (e.g., `--log-file /path/to/log`)
   B. Configuration file option only (add `log_file` to config.yaml)
   C. Both CLI flag and config file (CLI takes precedence)
   D. Other (please specify)

2. Should the file log output differ from stdout output?
   A. Same format (timestamps, prefixes, etc.)
   B. File logs should be more detailed (always include file/line info)
   C. File logs should include JSON/structured format for parsing
   D. Other (please specify)

3. How should log file rotation/management be handled?
   A. No rotation - single file that grows indefinitely
   B. Simple size-based rotation (e.g., rotate at 10MB, keep 5 files)
   C. Time-based rotation (daily/weekly)
   D. Leave rotation to external tools (logrotate) - just append to file
   E. Other (please specify)
