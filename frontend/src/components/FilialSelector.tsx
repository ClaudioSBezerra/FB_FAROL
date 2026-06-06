import { useState } from "react";
import { Check, ChevronsUpDown, GitBranch } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { useFiliais, Filial } from "@/contexts/FilialContext";
import { formatCNPJMasked } from "@/lib/formatFilial";

/** Rótulo exibido para uma filial: CNPJ mascarado + apelido se existir */
function filialLabel(f: Filial): string {
  const cnpj = formatCNPJMasked(f.cnpj);
  return f.apelido ? `${cnpj} - ${f.apelido}` : cnpj;
}

export function FilialSelector() {
  const { filiais, selectedFiliais, toggleFilial, selectAll } = useFiliais();
  const [open, setOpen] = useState(false);

  // Oculta o seletor quando há apenas 1 filial (sem utilidade filtrar)
  if (filiais.length <= 1) return null;

  // Rótulo do botão trigger
  const triggerLabel =
    selectedFiliais.length === 0
      ? "Todas as filiais"
      : selectedFiliais.length === 1
        ? filialLabel(filiais.find(f => f.cnpj === selectedFiliais[0]) ?? { cnpj: selectedFiliais[0], nome: "", apelido: "" })
        : `${selectedFiliais.length} filiais`;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className="h-8 rounded-lg border-slate-200 bg-white px-2 gap-1.5 text-xs text-slate-700 shadow-sm hover:bg-slate-50 font-normal"
        >
          <GitBranch className="h-3 w-3 shrink-0 text-slate-400" />
          <span className="truncate max-w-[140px]">{triggerLabel}</span>
          {selectedFiliais.length > 0 && (
            <Badge
              variant="secondary"
              className="ml-1 text-[9px] px-1 py-0 h-4 shrink-0"
            >
              {selectedFiliais.length}
            </Badge>
          )}
          <ChevronsUpDown className="h-3 w-3 shrink-0 opacity-40" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[280px] p-0" side="right" align="end">
        <Command>
          <CommandInput placeholder="Buscar filial..." className="h-7 text-xs" />
          <CommandList>
            <CommandEmpty className="text-xs py-2">Nenhuma filial encontrada.</CommandEmpty>
            <CommandGroup>
              {/* Opção "Todas" */}
              <CommandItem
                value="__todas__"
                onSelect={() => {
                  selectAll();
                  setOpen(false);
                }}
                className="py-1"
              >
                <Check
                  className={cn(
                    "mr-1.5 h-3.5 w-3.5 shrink-0",
                    selectedFiliais.length === 0 ? "opacity-100" : "opacity-0"
                  )}
                />
                <span className="text-xs font-medium">Todas as filiais</span>
              </CommandItem>

              {/* Lista de filiais */}
              {filiais.map(f => (
                <CommandItem
                  key={f.cnpj}
                  value={filialLabel(f) + f.cnpj}
                  onSelect={() => toggleFilial(f.cnpj)}
                  className="py-1"
                >
                  <Check
                    className={cn(
                      "mr-1.5 h-3.5 w-3.5 shrink-0",
                      selectedFiliais.includes(f.cnpj) ? "opacity-100" : "opacity-0"
                    )}
                  />
                  <span className="text-[11px] leading-tight">{filialLabel(f)}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
