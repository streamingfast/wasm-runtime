export function hello(name: string): string{
  // It appears not all standard JavaScript methods exists here, `toUpperCase` for example leads to a compilation error
  return "hello " + name
}