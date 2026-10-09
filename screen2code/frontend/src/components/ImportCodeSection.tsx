import { useState } from "react";
import { Button } from "./ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "./ui/dialog";
import { Textarea } from "./ui/textarea";
import OutputSettingsSection from "./settings/OutputSettingsSection";
import toast from "react-hot-toast";
import { Stack } from "../lib/stacks";

interface Props {
  importFromCode: (code: string, stack: Stack) => void;
}

function ImportCodeSection({ importFromCode }: Props) {
  const [code, setCode] = useState("");
  const [stack, setStack] = useState<Stack | undefined>(undefined);

  const doImport = () => {
    if (code === "") {
      toast.error("请先粘贴代码");
      return;
    }

    if (stack === undefined) {
      toast.error("请选择技术栈");
      return;
    }

    importFromCode(code, stack);
  };
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button className="import-from-code-btn" variant="secondary">

          导入代码
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>粘贴 HTML 代码</DialogTitle>
          <DialogDescription>

            请确保导入的是有效 HTML 代码。
          </DialogDescription>
        </DialogHeader>

        <Textarea
          value={code}
          onChange={(e) => setCode(e.target.value)}
          className="w-full h-64"
        />

        <OutputSettingsSection
          stack={stack}
          setStack={(config: Stack) => setStack(config)}
          label="技术栈："
          shouldDisableUpdates={false}
        />

        <DialogFooter>
          <Button className="import-btn" type="submit" onClick={doImport}>

            导入
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default ImportCodeSection;
