package server

import (
	"reflect"
	"testing"
)

func Test_storages_getHashTable(t *testing.T) {
	type fields struct {
		data map[string]storage
	}
	type args struct {
		key string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    map[string][]byte
		wantErr bool
	}{
		{
			name: "Success t:1000",
			fields: fields{
				data: map[string]storage{
					"t:1000": storage{
						expired: -1,
						vocabulary: map[string][]byte{
							"name": []byte("Dmitry"),
						}},
				},
			},
			args: args{key: "t:1000"},
			want: map[string][]byte{
				"name": []byte("Dmitry"),
			},
			wantErr: false,
		},
		{
			name: "Error Key Have Another Type t:1000",
			fields: fields{
				data: map[string]storage{
					"t:1000": storage{
						expired: -1,
						str:     []byte("Dmitry"),
					},
				},
			},
			args:    args{key: "t:1000"},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Error Key Have Another Type t:1000",
			fields: fields{
				data: map[string]storage{
					"t:1000": storage{
						expired: -1,
						str:     []byte("Dmitry"),
					},
				},
			},
			args:    args{key: "t:1000"},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Error Key Not Found t:1000",
			fields: fields{
				data: map[string]storage{},
			},
			args:    args{key: "t:1000"},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &storages{
				data: tt.fields.data,
			}
			got, err := st.getHashTable(tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("storages.getHashTable() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("storages.getHashTable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_storages_hset(t *testing.T) {
	type fields struct {
		data map[string]storage
	}
	type args struct {
		key   string
		field string
		value string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &storages{
				data: tt.fields.data,
			}
			if err := st.hset(tt.args.key, tt.args.field, tt.args.value); (err != nil) != tt.wantErr {
				t.Errorf("storages.hset() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_storages_hget(t *testing.T) {
	type fields struct {
		data map[string]storage
	}
	type args struct {
		key   string
		field string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []byte
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &storages{
				data: tt.fields.data,
			}
			got, err := st.hget(tt.args.key, tt.args.field)
			if (err != nil) != tt.wantErr {
				t.Errorf("storages.hget() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("storages.hget() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_storages_hgetall(t *testing.T) {
	type fields struct {
		data map[string]storage
	}
	type args struct {
		key string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []byte
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &storages{
				data: tt.fields.data,
			}
			got, err := st.hgetall(tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("storages.hgetall() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("storages.hgetall() = %v, want %v", got, tt.want)
			}
		})
	}
}
