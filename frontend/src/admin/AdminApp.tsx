import { getDictionary } from "../lib/i18n";
import type { AdminPageData } from "../types";
import { AdminCategories, AdminEdit, AdminList, AdminLogin, AdminPassword, AdminProfile, AdminSetup } from "./Admin";

export function AdminApp(props: AdminPageData) {
  switch (props.page) {
    case "adminLogin":
      return <AdminLogin {...props} />;
    case "adminSetup":
      return <AdminSetup {...props} />;
    case "adminPassword":
      return <AdminPassword {...props} />;
    case "adminList":
      return <AdminList {...props} />;
    case "adminEdit":
      return <AdminEdit {...props} />;
    case "adminProfile":
      return <AdminProfile {...props} />;
    case "adminCategories":
      return <AdminCategories {...props} />;
  }
}

export function adminHead(props: AdminPageData): { title: string; description: string } {
  const site = `${getDictionary(props.lang).siteTitle} Admin`;
  switch (props.page) {
    case "adminLogin":
      return { title: `Sign in – ${site}`, description: "" };
    case "adminSetup":
      return { title: `Set up – ${site}`, description: "" };
    case "adminPassword":
      return { title: `Password – ${site}`, description: "" };
    case "adminList":
      return { title: `Articles – ${site}`, description: "" };
    case "adminEdit":
      return { title: `${props.mode === "new" ? "New article" : `Edit: ${props.form.title}`} – ${site}`, description: "" };
    case "adminProfile":
      return { title: `Profile – ${site}`, description: "" };
    case "adminCategories":
      return { title: `Categories – ${site}`, description: "" };
  }
}
