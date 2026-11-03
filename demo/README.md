# Demo boards

These three boards illustrate a request arriving, a draft reply, human approval,
and a CRM update. Each uses sketch style and six presentation steps. Choose
`landing-en.yaml`, `landing-uk.yaml`, or `landing-ru.yaml`.

```sh
kreida set demo < demo/landing-en.yaml
kreida open demo
kreida png demo -o demo.png
```

Use `--step N` with `kreida png` to render one step.
