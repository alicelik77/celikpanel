import { lazy, useState } from 'react';
import { useLocation, useNavigate, useParams } from '../router';

// One component's management page (/components/:serviceId), out of App.tsx
// since alpha.134 so the boot path does not carry it (App.tsx loads this
// module lazily). Each specialised page stays its own chunk.
// Bileşen yönetim sayfası; alpha.134'ten beri App.tsx dışında, tembel yüklenir.
const ConfigEditor = lazy(() => import('./ConfigEditor').then((module) => ({ default: module.ConfigEditor })));
const PHPManagement = lazy(() => import('./PHPManagement').then((module) => ({ default: module.PHPManagement })));
const NginxManagement = lazy(() => import('./NginxManagement').then((module) => ({ default: module.NginxManagement })));
const Fail2banManagement = lazy(() => import('./Fail2banManagement').then((module) => ({ default: module.Fail2banManagement })));
const PostfixManagement = lazy(() => import('./PostfixManagement').then((module) => ({ default: module.PostfixManagement })));
const DovecotManagement = lazy(() => import('./DovecotManagement').then((module) => ({ default: module.DovecotManagement })));
const PowerDNSManagement = lazy(() => import('./PowerDNSManagement').then((module) => ({ default: module.PowerDNSManagement })));
const VsftpdManagement = lazy(() => import('./VsftpdManagement').then((module) => ({ default: module.VsftpdManagement })));
const PostgreSQLManagement = lazy(() => import('./PostgreSQLManagement').then((module) => ({ default: module.PostgreSQLManagement })));
const MariaDBManagement = lazy(() => import('./MariaDBManagement').then((module) => ({ default: module.MariaDBManagement })));
const ComponentDetail = lazy(() => import('./ComponentDetail').then((module) => ({ default: module.ComponentDetail })));
const ServiceRecordLookup = lazy(() => import('./ServiceRecordLookup').then((module) => ({ default: module.ServiceRecordLookup })));

// Service Management Wrapper
interface ServiceManagementProps {
  serviceId: string;
  versions: string[];
  onSelectConfig?: (path: string) => void;
}

function ServiceManagement({ serviceId, versions, onSelectConfig }: ServiceManagementProps) {
  const navigate = useNavigate();
  const onBack = () => navigate('/components');

  switch (serviceId) {
    case 'php-fpm':
      return <PHPManagement versions={versions} onBack={onBack} />;
    case 'nginx':
      return <NginxManagement onBack={onBack} />;
    case 'fail2ban':
      return <Fail2banManagement onBack={onBack} />;
    case 'postfix':
      return <PostfixManagement onBack={onBack} />;
    case 'dovecot':
      return <DovecotManagement onBack={onBack} onSelectConfig={onSelectConfig} />;
    case 'pdns':
      return <PowerDNSManagement onBack={onBack} />;
    case 'vsftpd':
      return <VsftpdManagement onBack={onBack} onSelectConfig={onSelectConfig} />;
    case 'postgresql':
      return <PostgreSQLManagement onBack={onBack} />;
    case 'mariadb':
      return <MariaDBManagement onBack={onBack} />;
    default:
      // Every other component gets the DERIVED page — status, actions, unit,
      // versions, packages, ports, config files and its own journal — instead
      // of the dead end that used to sit here (operator, 25 Jul: "birçok
      // servisin manage'i doğru düzgün çalışmıyor"). The specialised pages
      // above stay because they do more than describe; this one needs no entry
      // in any list, so a component added tomorrow is manageable at once.
      // Geri kalan her bileşen, burada eskiden duran çıkmaz sokak yerine
      // TÜRETİLMİŞ sayfayı alır: durum, eylemler, unit, sürümler, paketler,
      // portlar, ayar dosyaları ve kendi günlüğü (operatör, 25 Tem: "birçok
      // servisin manage'i doğru düzgün çalışmıyor"). Yukarıdaki özel sayfalar
      // betimlemekten fazlasını yaptıkları için kalır; bunun hiçbir listeye
      // girmesi gerekmez, yani yarın eklenen bileşen anında yönetilebilir.
      return <ComponentDetail serviceId={serviceId} onBack={onBack} onSelectConfig={onSelectConfig} />;
  }
}



export function ServiceManagementPage() {
  const { serviceId } = useParams();
  const navigate = useNavigate();
  const location = useLocation();
  const navigationState = location.state as { versions?: string[] } | null;
  // Config files listed on the generic page open in the same editor the
  // Components page uses — one editor, not a second copy.
  // Genel sayfada listelenen ayar dosyaları, Bileşenler sayfasının kullandığı
  // editörde açılır — tek editör, ikinci bir kopya değil.
  const [configPath, setConfigPath] = useState<string | null>(null);

  // No empty-versions bailout: with the "default" sentinel dead (B3b), an
  // installed nginx/postfix legitimately has versions: [] — bailing out here
  // rendered a BLANK page behind every such Manage click.
  // Boş-sürüm kaçışı yok: "default" sentinel'i öldüğünden (B3b) kurulu bir
  // nginx/postfix meşru olarak versions: [] taşır — burada kaçmak, böyle her
  // Yönet tıklamasının arkasında BOŞ sayfa çiziyordu.
  //
  // After a reload the versions are not in the navigation any more; the lookup
  // reads them and stays on this address whatever that read says.
  // Yeniden yüklemeden sonra sürümler gezinmede yoktur; arama onları okur ve
  // okuma ne derse desin bu adreste kalır.
  return (
    <ServiceRecordLookup
      serviceId={serviceId ?? ''}
      carriedVersions={navigationState?.versions}
      onBack={() => navigate('/components')}
    >
      {(versions) => (configPath
        ? <ConfigEditor path={configPath} onBack={() => setConfigPath(null)} />
        : <ServiceManagement serviceId={serviceId!} versions={versions} onSelectConfig={setConfigPath} />)}
    </ServiceRecordLookup>
  );
}
