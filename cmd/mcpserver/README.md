Below is a list of example prompts for each tool. Each example is a JSON snippet showing one way you might call the tool:

---

**agent**  
```json
{
  "maxCalls": 3,
  "query": "What are the top technology news headlines today?"
}
```

---

**calculate**  
```json
{
  "a": 10,
  "b": 5,
  "operation": "subtract"
}
```

---

**copy_file**  
```json
{
  "source": "/path/to/source/file.txt",
  "destination": "/path/to/destination/file.txt",
  "recursive": false
}
```

---

**create_directory**  
```json
{
  "path": "/tmp/new_project_folder"
}
```

---

**delete_file**  
```json
{
  "path": "/tmp/old_config.yaml",
  "recursive": false
}
```

---

**directory_tree**  
```json
{
  "path": "/home/user/projects",
  "maxDepth": 2
}
```

---

**edit_file**  
```json
{
  "path": "/home/user/app/config.json",
  "search": "debug:true",
  "replace": "debug:false"
}
```

---

**format_go_code**  
```json
{
  "path": "/home/user/go/src/myapp"
}
```

---

**get_file_info**  
```json
{
  "path": "/etc/hosts"
}
```

---

**get_weather**  
```json
{
  "latitude": 40.7128,
  "longitude": -74.0060
}
```

---

**git_add**  
```json
{
  "path": "/home/user/repo",
  "fileList": ["main.go", "README.md"]
}
```

---

**git_checkout**  
```json
{
  "path": "/home/user/repo",
  "branch": "feature/new-api",
  "createNew": true
}
```

---

**git_clone**  
```json
{
  "repoUrl": "https://github.com/example/repo.git",
  "path": "/home/user/projects/repo"
}
```

---

**git_commit**  
```json
{
  "path": "/home/user/repo",
  "message": "Fix authentication bug"
}
```

---

**git_diff**  
```json
{
  "path": "/home/user/repo",
  "fromRef": "main",
  "toRef": "feature/updates"
}
```

---

**git_init**  
```json
{
  "path": "/home/user/new_project"
}
```

---

**git_pull**  
```json
{
  "path": "/home/user/repo"
}
```

---

**git_push**  
```json
{
  "path": "/home/user/repo"
}
```

---

**git_status**  
```json
{
  "path": "/home/user/repo"
}
```

---

**go_build**  
```json
{
  "path": "/home/user/go/src/myapp"
}
```

---

**go_test**  
```json
{
  "path": "/home/user/go/src/myapp"
}
```

---

**hello**  
```json
{
  "name": "Alice"
}
```

---

**lint_code**  
```json
{
  "path": "/home/user/project",
  "linterName": "golangci-lint"
}
```

---

**list_allowed_directories**  
```json
{}
```

---

**list_directory**  
```json
{
  "path": "/home/user/documents"
}
```

---

**move_file**  
```json
{
  "source": "/home/user/old_name.txt",
  "destination": "/home/user/new_name.txt"
}
```

---

**read_file**  
```json
{
  "path": "/home/user/README.md"
}
```

---

**read_multiple_files**  
```json
{
  "paths": [
    "/home/user/config.json",
    "/home/user/data.json"
  ]
}
```

---

**run_shell_command**  
```json
{
  "directory": "/home/user",
  "command": ["ls", "-la"]
}
```

---

**search_files**  
```json
{
  "path": "/var/log",
  "pattern": "ERROR"
}
```

---

**time**  
```json
{
  "format": "RFC3339"
}
```

---

**write_file**  
```json
{
  "path": "/home/user/hello.txt",
  "content": "Hello, World!"
}
```

---

Each of these prompts matches the tool's JSON schema and demonstrates how you might invoke the tool with typical data. Feel free to modify the examples as needed to suit your specific use case!