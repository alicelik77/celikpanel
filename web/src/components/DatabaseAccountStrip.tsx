// alpha.133: CelikPanel's account on a database engine moved to the MariaDB
// and PostgreSQL pages; its code is in ./DatabaseRemoteServerPanel.tsx (one
// lazily loaded part with Security > Open to the outside). This file only
// keeps the Databases page's import working until that page stops showing the
// account (lane dblist-fe); it is deleted in the same integration.
// alpha.133: hesap bileşen sayfalarına taşındı; bu dosya yalnız Veritabanları
// sayfasının içe aktarımını birleştirmeye dek ayakta tutar.
export { DatabaseAccountStrip } from './DatabaseRemoteServerPanel';
