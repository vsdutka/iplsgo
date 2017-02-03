// describe
package otasker

import (
	"github.com/vsdutka/metrics"
	"gopkg.in/errgo.v1"
	"gopkg.in/goracle.v1/oracle"
	"strings"
	"sync"
	"time"
)

var (
	describeTotalTime = metrics.NewInt("Describe_Total_Time", "Describe - Total time in nanoseconds", "Nanoseconds", "ns")

	describeRLockWaitTime    = metrics.NewInt("Describe_RLock_Wait_Time", "Describe - Wait time to RLock in nanoseconds", "Nanoseconds", "ns")
	describeRLockWaitTimes   = metrics.NewInt("Describe_RLock_Wait_Times", "Describe - Total number of Wait to RLock", "pieces", "ps")
	describeRLockWaitNum     = metrics.NewInt("Describe_RLock_Wait_Nums", "Describe - Current number of Wait to RLock", "pieces", "ps")
	describeRLockWaitTimeAve = metrics.NewFloat("Describe_RLock_Wait_Time_Ave", "Describe - Average Wait time to RLock in nanoseconds", "Nanoseconds", "ns")

	describeLockWaitTime    = metrics.NewInt("Describe_Lock_Wait_Time", "Describe - Wait time to Lock in nanoseconds", "Nanoseconds", "ns")
	describeLockWaitTimes   = metrics.NewInt("Describe_Lock_Wait_Times", "Describe - Total number of Wait to Lock", "pieces", "ps")
	describeLockWaitNum     = metrics.NewInt("Describe_Lock_Wait_Nums", "Describe - Current number of Wait to Lock", "pieces", "ps")
	describeLockWaitTimeAve = metrics.NewFloat("Describe_Lock_Wait_Time_Ave", "Describe - Average Wait time to Lock in nanoseconds", "Nanoseconds", "ns")
)

type argument struct {
	dataType     int32
	dataTypeName string
}
type procedure struct {
	timestamp   time.Time
	packageName string
	arguments   map[string]*argument
}

var (
	plock sync.RWMutex
	plist = make(map[string]*procedure)
	aFree = sync.Pool{
		New: func() interface{} {
			return new(argument)
		},
	}
)

func doRLock() {
	describeRLockWaitNum.Add(1)
	bg := time.Now()
	plock.RLock()
	tm := time.Since(bg).Nanoseconds()
	describeRLockWaitNum.Add(-1)
	describeRLockWaitTime.Add(tm)
	describeRLockWaitTimes.Add(1)
	describeRLockWaitTimeAve.Set(float64(describeRLockWaitTime.Get()) / float64(describeRLockWaitTimes.Get()))
}
func doRUnlock() {
	plock.RUnlock()
}
func doLock() {
	describeLockWaitNum.Add(1)
	bg := time.Now()
	plock.Lock()
	tm := time.Since(bg).Nanoseconds()
	describeLockWaitNum.Add(-1)
	describeLockWaitTime.Add(tm)
	describeLockWaitTimes.Add(1)
	describeLockWaitTimeAve.Set(float64(describeLockWaitTime.Get()) / float64(describeLockWaitTimes.Get()))
}
func doUnlock() {
	plock.Unlock()
}

func ProcedureInfo(dbName, procedureName string) (time.Time, string, error) {
	doRLock()
	defer doRUnlock()
	if p, ok := plist[strings.ToUpper(dbName+"."+procedureName)]; ok {
		return p.timestamp, p.packageName, nil
	}
	return time.Time{}, "", errgo.Newf("Отсутствует описание для процедуры \"%s\"\n", procedureName)
}

func ArgumentInfo(dbName, procedureName, argumentName string) (int32, string, error) {
	doRLock()
	defer doRUnlock()
	if p, ok := plist[strings.ToUpper(dbName+"."+procedureName)]; ok {
		if a, ok := p.arguments[strings.ToUpper(argumentName)]; ok {
			return a.dataType, a.dataTypeName, nil
		}
		return 0, "", errgo.Newf("Отсутствует описание аргумента \"%s\" для процедуры \"%s\"\n", argumentName, procedureName)
	}
	return 0, "", errgo.Newf("Отсутствует описание для процедуры \"%s\"\n", procedureName)
}

func Describe(conn *oracle.Connection, dbName, procedureName string) error {
	var (
		err            error
		arrayLen       int32
		timestamp      time.Time
		packageName    string
		parsedProcName string
		objectId       int32
		shouldDescribe bool
	)
	bg := time.Now()
	defer describeTotalTime.Add(time.Since(bg).Nanoseconds())
	timestamp, packageName, err = ProcedureInfo(dbName, procedureName)

	//ВСЕГДА проверяем были ли изменения и получаем размер массивов для информации по параметрам
	shouldDescribe, arrayLen, err = func() (bool, int32, error) {
		var (
			updated           int32
			procedureNameVar  *oracle.Variable
			updatedVar        *oracle.Variable
			arrayLenVar       *oracle.Variable
			lastChangeTimeVar *oracle.Variable
			parsedProcNameVar *oracle.Variable
			objectIdVar       *oracle.Variable
			packageNameVar    *oracle.Variable
		)
		curShort := conn.NewCursor()
		defer curShort.Close()

		if procedureNameVar, err = curShort.NewVar(&procedureName); err != nil {
			return false, 0, errgo.Newf("error creating variable for %s(%T): %s", procedureName, procedureName, err)
		}
		defer procedureNameVar.Free()

		if packageNameVar, err = curShort.NewVar(&packageName); err != nil {
			return false, 0, errgo.Newf("error creating variable for %s(%T): %s", packageName, packageName, err)
		}
		defer packageNameVar.Free()

		if parsedProcNameVar, err = curShort.NewVar(&parsedProcName); err != nil {
			return false, 0, errgo.Newf("error creating variable for %s(%T): %s", parsedProcName, parsedProcName, err)
		}
		defer parsedProcNameVar.Free()

		if objectIdVar, err = curShort.NewVar(&objectId); err != nil {
			return false, 0, errgo.Newf("error creating variable for %s(%T): %s", objectId, objectId, err)
		}
		defer objectIdVar.Free()

		if lastChangeTimeVar, err = curShort.NewVar(&timestamp); err != nil {
			return false, 0, errgo.Newf("error creating variable for %s(%T): %s", timestamp, timestamp, err)
		}
		defer lastChangeTimeVar.Free()

		if updatedVar, err = curShort.NewVar(&updated); err != nil {
			return false, 0, errgo.Newf("error creating variable for %s(%T): %s", updated, updated, err)
		}
		defer updatedVar.Free()

		if arrayLenVar, err = curShort.NewVar(&arrayLen); err != nil {
			return false, 0, errgo.Newf("error creating variable for %s(%T): %s", arrayLen, arrayLen, err)
		}
		defer arrayLenVar.Free()

		if err := curShort.Execute(stm_descr_short, nil, map[string]interface{}{"proc_name": procedureNameVar,
			"package_name":   packageNameVar,
			"last_ddl_time":  lastChangeTimeVar,
			"updated":        updatedVar,
			"len_":           arrayLenVar,
			"procedure_name": parsedProcNameVar,
			"object_id":      objectIdVar,
		}); err != nil {
			return false, 0, err
			//return false, 0, errgo.Newf("Невозможно получить описание для \"%s\"\nОшибка: %s", procedureName, err.Error())
		}

		if updated == 1 {
			return true, arrayLen, nil
		}
		return false, 0, nil
	}()
	if err != nil {
		return err
	}

	if shouldDescribe {
		err = func() error {
			curLong := conn.NewCursor()
			defer curLong.Close()
			if err := curLong.Execute(stm_descr_long, []interface{}{objectId, parsedProcName}, nil); err != nil {
				return err
				//return errgo.Newf("Невозможно получить описание для \"%s\"\nОшибка: %s", procedureName, err.Error())
			}
			rows, err := curLong.FetchAll()
			if err != nil {
				return err
				//return errgo.Newf("Невозможно получить описание для \"%s\"\nОшибка: %s", procedureName, err.Error())
			}

			doLock()
			defer doUnlock()

			p, ok := plist[strings.ToUpper(dbName+"."+procedureName)]
			if !ok {
				p = &procedure{arguments: make(map[string]*argument)}
				plist[strings.ToUpper(dbName+"."+procedureName)] = p
			} else {
				for k := range p.arguments {
					aFree.Put(p.arguments[k])
					delete(p.arguments, k)
				}
			}
			p.timestamp = timestamp
			p.packageName = packageName

			for _, row := range rows {
				a := aFree.Get()
				argumentInstance := a.(*argument)
				argumentInstance.dataType = row[1].(int32)
				argumentInstance.dataTypeName = row[2].(string)
				p.arguments[row[0].(string)] = argumentInstance
			}
			return nil
		}()
		return err
	}
	return nil
}

const (
	oString     = 1
	oNumber     = 2
	oDate       = 3
	oBoolean    = 4
	oInteger    = 5
	oStringTab  = 11
	oNumberTab  = 12
	oDateTab    = 13
	oBooleanTab = 14
	oIntegerTab = 15
)
const (
	stm_descr_args = `
from 
    all_arguments a
    ,all_arguments sa
  where a.data_level = 0
  and a.argument_name is not null
  and sa.OBJECT_ID(+) = a.OBJECT_ID
  and sa.SUBPROGRAM_ID(+) = a.SUBPROGRAM_ID
  and sa.DATA_LEVEL(+) = a.DATA_LEVEL + 1
  and sa.SEQUENCE(+) = a.SEQUENCE + 1 
  and
    (
      a.pls_type in ('CHAR', 'DATE', 'FLOAT', 'NUMBER', 'VARCHAR2', 'STRING', 'BOOLEAN', 'INTEGER', 'PLS_INTEGER', 'DECIMAL')
      or
      (
        a.DATA_TYPE = 'PL/SQL TABLE'
        and
        sa.pls_type in ('CHAR', 'DATE', 'FLOAT', 'NUMBER', 'VARCHAR2', 'STRING', 'BOOLEAN', 'INTEGER', 'PLS_INTEGER', 'DECIMAL')
      )
    )`

	stm_descr_short = `declare
  lstatus varchar2(40);
  lschema VARCHAR2(40);
  lpart1 VARCHAR2(40);
  lpart2 VARCHAR2(40);
  ldblink VARCHAR2(40);
  lpart1_type NUMBER;
  lobject_type VARCHAR2(40);
  llast_ddl_time date;
  ex1 exception;
  pragma exception_init(ex1, -06564);
begin
  DBMS_UTILITY.NAME_RESOLVE(:proc_name,1,lschema,lpart1,lpart2,ldblink,lpart1_type,:object_id);
  if lpart1_type = 9 then
    :package_name := lschema || '.' || lpart1;
  else
    :package_name := null;
  end if;
  
  select status, object_type, last_ddl_time
  into lstatus, lobject_type, llast_ddl_time
  from all_objects
  where all_objects.object_id=:object_id;
  if lstatus='INVALID' then
    dbms_ddl.alter_compile(lobject_type,lschema,nvl(lpart1,lpart2));
    llast_ddl_time := sysdate;
  end if;
  if llast_ddl_time <= :last_ddl_time then
    :updated := 0;
    :len_ := 0;
  else
    :updated := 1;
    :last_ddl_time := llast_ddl_time;
    
    if lpart1_type = 9 then
      :package_name := lschema || '.' || lpart1;
    else
      :package_name := null;
    end if;
    :procedure_name := lpart2;
    
    select count(*)
    into :len_` + stm_descr_args + `
    and a.object_id = :object_id
    and a.object_name = lpart2;
  end if;
  commit;
exception
  when others then
    rollback;
    if sqlcode in (-20000, -20001, -20002, -20003, -20004) then
      raise ex1;
    else
       raise;
    end if;
end;`
	stm_descr_long = `select 
          a.ARGUMENT_NAME name,
          case
            when a.pls_type in ('CHAR', 'VARCHAR2', 'STRING') then 1
            when a.pls_type in ('FLOAT', 'NUMBER', 'DECIMAL') then 2
            when a.pls_type in ('DATE') then 3
            when a.pls_type in ('BOOLEAN') then 4
			when a.pls_type in ('INTEGER', 'PLS_INTEGER') then 5
            when a.DATA_TYPE = 'PL/SQL TABLE' then
              case
                when sa.pls_type in ('CHAR', 'VARCHAR2', 'STRING') then 11
                when sa.pls_type in ('FLOAT', 'NUMBER', 'DECIMAL') then 12
                when sa.pls_type in ('DATE') then 13
                when sa.pls_type in ('BOOLEAN') then 14
				when sa.pls_type in ('INTEGER', 'PLS_INTEGER') then 15
                else 0
              end
            else 0
          end data_type,
         
          case 
            when a.type_name is not null then a.type_owner||'.'||a.type_name||decode(a.type_subname, null, '', '.'||a.type_subname) 
			/*when a.pls_type in ('CHAR', 'VARCHAR2', 'STRING') then a.pls_type||'('||nvl(a.char_length, 32767)||')'*/
            else a.pls_type
          end data_type_name` + stm_descr_args + `
        and a.object_id = :1
        and a.object_name = :2
`
)
