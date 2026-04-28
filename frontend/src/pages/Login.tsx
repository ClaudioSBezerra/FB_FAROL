import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { toast } from "sonner";
import { useNavigate, Link } from "react-router-dom";
import { useAuth } from "@/contexts/AuthContext";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { AlertCircle, Target, TrendingUp, Users, Trophy } from "lucide-react";

const FEATURES = [
  { icon: Target,     text: "Objetivos de venda por fornecedor e período" },
  { icon: TrendingUp, text: "Acompanhamento de resultados em tempo real" },
  { icon: Users,      text: "Gestão hierárquica de RCAs e supervisores" },
  { icon: Trophy,     text: "Rankings e metas por equipe e território" },
];

const Login = () => {
  const [email, setEmail]       = useState("");
  const [password, setPassword] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [errorMsg, setErrorMsg]   = useState<string | null>(null);
  const navigate = useNavigate();
  const { login } = useAuth();

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setErrorMsg(null);

    try {
      const res = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password }),
      });

      const data = await res.json();

      if (!res.ok) {
        throw new Error(typeof data === "string" ? data : "Credenciais inválidas");
      }

      login(data);
      toast.success("Login realizado com sucesso!");
      navigate("/cadastros/rcas");
    } catch (error: unknown) {
      const msg = error instanceof Error ? error.message : "Erro desconhecido";
      setErrorMsg(msg);
      toast.error(msg);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex">

      {/* ── Painel esquerdo — identidade Farol ───────────────────────── */}
      <div
        className="hidden lg:flex lg:w-2/5 flex-col justify-between p-10 relative overflow-hidden"
        style={{ background: "#0f172a" }}
      >
        {/* Círculos decorativos */}
        <div className="absolute -top-24 -right-24 w-96 h-96 rounded-full pointer-events-none"
          style={{ background: "radial-gradient(circle, #1e3a5f 0%, #0f172a 100%)" }} />
        <div className="absolute -bottom-20 -left-20 w-64 h-64 rounded-full pointer-events-none"
          style={{ background: "radial-gradient(circle, #162032 0%, #0f172a 100%)" }} />

        {/* ── Topo: logotipo do produto ── */}
        <div className="relative z-10">
          <div className="flex items-center gap-3 mb-12">
            <div className="w-10 h-10 rounded-xl flex items-center justify-center"
              style={{ background: "#2563eb" }}>
              <Target className="w-6 h-6 text-white" />
            </div>
            <span className="text-white text-xl font-bold tracking-tight">Farol</span>
          </div>

          {/* Badge */}
          <span className="inline-block px-4 py-1.5 rounded-full text-sm uppercase tracking-widest font-semibold"
            style={{
              background: "rgba(37,99,235,0.15)",
              color: "#93c5fd",
              border: "1px solid rgba(37,99,235,0.3)",
            }}>
            Gestão Inteligente de Vendas
          </span>

          {/* Título */}
          <h1 className="text-white text-4xl font-bold leading-tight mt-5">
            Acompanhe metas.
            <br />
            <span style={{ color: "#60a5fa" }}>Acelere resultados.</span>
          </h1>

          {/* Subtítulo */}
          <p className="mt-5 text-base leading-relaxed" style={{ color: "#94a3b8" }}>
            Gestão inteligente de objetivos de vendas e resultados — por RCA,
            fornecedor e período — com visibilidade completa para toda a hierarquia comercial.
          </p>
        </div>

        {/* ── Rodapé: features ── */}
        <div className="relative z-10 space-y-4">
          <ul className="space-y-3">
            {FEATURES.map(({ icon: Icon, text }) => (
              <li key={text} className="flex items-center gap-3 text-sm" style={{ color: "#cbd5e1" }}>
                <div className="w-6 h-6 rounded-md flex items-center justify-center shrink-0"
                  style={{ background: "rgba(37,99,235,0.2)" }}>
                  <Icon className="w-3.5 h-3.5" style={{ color: "#60a5fa" }} />
                </div>
                {text}
              </li>
            ))}
          </ul>

          <p className="text-xs pt-2" style={{ color: "#475569" }}>
            © {new Date().getFullYear()} Fortes Bezerra Tecnologia · Farol v{__APP_VERSION__}
          </p>
        </div>
      </div>

      {/* ── Painel direito — formulário de login ─────────────────────────── */}
      <div className="flex-1 flex items-center justify-center bg-gray-50 px-4">
        <div className="w-full max-w-[420px]">

          {/* Logo mobile (só aparece em telas pequenas) */}
          <div className="flex lg:hidden items-center justify-center gap-2 mb-8">
            <div className="w-8 h-8 rounded-lg flex items-center justify-center"
              style={{ background: "#2563eb" }}>
              <Target className="w-5 h-5 text-white" />
            </div>
            <span className="text-lg font-bold text-gray-900">Farol</span>
          </div>

          <Card className="w-full shadow-md border-0">
            <CardHeader className="flex flex-col items-center gap-1 space-y-0 pt-7 pb-4">
              <CardTitle className="text-base font-semibold">Acesse sua conta</CardTitle>
              <CardDescription className="text-xs">
                Entre com suas credenciais para continuar
              </CardDescription>
            </CardHeader>

            <CardContent>
              {errorMsg && (
                <Alert variant="destructive" className="mb-4">
                  <AlertCircle className="h-4 w-4" />
                  <AlertTitle>Erro</AlertTitle>
                  <AlertDescription>{errorMsg}</AlertDescription>
                </Alert>
              )}

              <form onSubmit={handleLogin} className="space-y-4">
                <div className="space-y-1.5">
                  <Label htmlFor="email" className="text-sm">E-mail</Label>
                  <Input
                    id="email"
                    type="email"
                    required
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="seu@email.com"
                    className="text-sm"
                  />
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="password" className="text-sm">Senha</Label>
                  <Input
                    id="password"
                    type="password"
                    required
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className="text-sm"
                  />
                </div>

                <div className="flex justify-end">
                  <Link to="/forgot-password" className="text-xs text-primary hover:underline">
                    Esqueci minha senha
                  </Link>
                </div>

                <Button type="submit" className="w-full text-sm" disabled={isLoading}>
                  {isLoading ? "Entrando..." : "Entrar"}
                </Button>
              </form>
            </CardContent>
          </Card>
        </div>
      </div>

    </div>
  );
};

export default Login;
