# Specialists & Routing

manifold supports routing requests to specialized OpenAI-compatible endpoints based on configurable patterns. This allows you to use different models or providers for specific types of requests.

## Configuration

Configure specialists in your `config.yaml`:

```yaml
specialists:
  - name: code_specialist
    baseURL: "https://api.example.com/v1"
    model: "code-model"
    apiKey: "specialist-api-key"
    # Optional headers
    headers:
      X-Custom-Header: "value"

routing:
  - pattern: "code|programming|function|class"
    specialist: code_specialist
    allowTools: ["run_cli", "write_file"]
  - pattern: "math|calculate|equation"
    specialist: math_specialist
    allowTools: ["calculator"]
```

## Routing Patterns

- Patterns are matched against user queries using regular expressions
- First matching pattern is used
- If no pattern matches, the default OpenAI configuration is used

## Tool Allow-listing

Each route can specify which tools are available:
- `allowTools`: List of tool names to allow for this route
- If not specified, all tools are available
- This provides fine-grained control over tool access per specialist

## Example Use Cases

1. **Code Generation**: Route programming requests to a code-specialized model
2. **Math Problems**: Route mathematical queries to a math-focused endpoint
3. **Domain Expertise**: Route domain-specific questions to specialized models
4. **Cost Optimization**: Route simple queries to cheaper models, complex ones to premium models