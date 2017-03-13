// owaekb
package otasker

func NewOwaEkbProcRunner() func() OracleTasker {
	return func() OracleTasker {
		return newTaskerIntf(ekbEvalSessionID, ekbMain, ekbGetRestChunk, ekbKillSession, ekbFileUpload)
	}
}

func NewOwaEkbProcTasker() func() oracleTasker {
	return func() oracleTasker {
		return newTasker(ekbEvalSessionID, ekbMain, ekbGetRestChunk, ekbKillSession, ekbFileUpload)
	}
}

const (
	ekbEvalSessionID = `select wskill_session.e_gcurrent_session_id from dual`
	ekbMain          = `
Declare
  rc__ number(2,0);
  l_num_params number;
  l_param_name wscontext.et_vc_arr;
  l_param_val wscontext.et_vc_arr;
  l_num_ext_params number;
  l_ext_param_name wscontext.et_vc_arr;
  l_ext_param_val wscontext.et_vc_arr;
  l_package_name varchar(240);
%s
Begin
%s  
  /* >> Инициализация параметров */
%s
  /* << Инициализация параметров */
  %s
  wscontext.e_init_cgi_env(l_num_params, l_param_name, l_param_val);
  wscontext.e_store_external_parameters(l_package_name, l_num_ext_params, l_ext_param_name, l_ext_param_val);
%s
  %s(%s);
  %s
  if (wsp.e_gIsFileDownload) then
    rc__ := 1;
    :content__ := '';

    declare
      l_lob blob := :lob__;
    begin
      :ContentType := '';
      :ContentLength := wsp.e_gContentLength;
      :CustomHeaders := wsp.e_gHTMLHdrs;
      wsp.e_Download_blob(l_lob);
      :lob__ := l_lob;
      :bNextChunkExists := 0;
    end;
    commit;
    dbms_session.modify_package_state(dbms_session.reinitialize);
  else
    rc__ := 0;
    commit;
    :ContentType := '';
    :ContentLength := wsp.e_gContentLength;
    :CustomHeaders := wsp.e_gHTMLHdrs;
    :content__ := wsp.e_gContentChunk(32000, :bNextChunkExists);
    if :bNextChunkExists = 0 then
      dbms_session.modify_package_state(dbms_session.reinitialize);
    end if;
  end if;
  commit;
  :rc__ := rc__;
  :sqlerrcode := 0;
  :sqlerrm := '';
  :sqlerrtrace := '';
exception
  when others then
    rollback;
    :sqlerrcode := SQLCODE;
    :sqlerrm := sqlerrm;
    :sqlerrtrace := DBMS_UTILITY.FORMAT_ERROR_BACKTRACE();
end;`

	ekbGetRestChunk = `begin
  :Data:=wsp.e_gContentChunk(32000, :bNextChunkExists);
  if :bNextChunkExists = 0 then
    dbms_session.modify_package_state(dbms_session.reinitialize);
  end if;
  commit;
  :sqlerrcode := 0;
  :sqlerrm := '';
  :sqlerrtrace := '';
exception
  when others then
    rollback;
    :sqlerrcode := SQLCODE;
    :sqlerrm := sqlerrm;
    :sqlerrtrace := DBMS_UTILITY.FORMAT_ERROR_BACKTRACE();
end;`
	ekbKillSession = `
begin
  wskill_session.ev_Session_ID:=:sess_id;
  :ret:=wskill_session.e_kill_session_by_session_id(:out_err_msg);
exception
  when others then
    if sqlcode = -00031 then
	  :ret := 1;
	else
      :ret := 0;
      :out_err_msg := sqlerrm;
	end if;
end;
`
	ekbFileUpload = `
declare
  l_item_id varchar2(40) := :item_id;/*Для совместимости*/
  l_application_id varchar2(40) := :application_id;/*Для совместимости*/
  l_page_id varchar2(40) := :page_id;/*Для совместимости*/
  l_session_id varchar2(40) := :session_id;/*Для совместимости*/
  l_request varchar2(40) := :request;/*Для совместимости*/
  l_mime_type varchar2(240) := :mime_type;/*Для совместимости*/
begin
  owa.init_cgi_env(:num_params, :param_name, :param_val);
  %s
  insert into %s(name, doc_size, last_updated, content_type, blob_content, PTDCD_ID)
  values(:name, :doc_size, sysdate, :content_type, :lob, pt_dc_by_user());
  :ret_name := :name;
  :sqlerrcode := 0;
  :sqlerrm := '';
  :sqlerrtrace := '';
exception
  when others then
    rollback;
    :sqlerrcode := -20000;
    :sqlerrm := 'Unable to upload file "'||:name||'" '||sqlerrm;
    :sqlerrtrace := DBMS_UTILITY.FORMAT_ERROR_BACKTRACE();
end;`
)
